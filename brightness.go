//go:build windows

package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// Brightness: external monitors via DDC/CI (dxva2), laptop panels via WMI (persistent PowerShell).

type physicalMonitor struct {
	Handle      uintptr
	Description [128]uint16
}

type ddcMonitor struct {
	h    uintptr
	max  uint32
	name string
}

var (
	brDelta  atomic.Int32
	brCycleF atomic.Bool
	brSignal = make(chan struct{}, 1)

	brMu      sync.Mutex
	ddcMons   []ddcMonitor
	ddcEnumAt time.Time
	brCurrent = -1
	brLastOp  time.Time
	hmonitors []uintptr
	monEnumCB = syscall.NewCallback(func(hmon, hdc, lprc, data uintptr) uintptr {
		hmonitors = append(hmonitors, hmon)
		return 1
	})

	wmi        *psHost
	wmiChecked bool
	wmiCount   int
)

var brPresets = []int{10, 40, 70, 100}

func brightnessAdd(d int) {
	brDelta.Add(int32(d))
	select {
	case brSignal <- struct{}{}:
	default:
	}
}

func brightnessCycle() {
	brCycleF.Store(true)
	select {
	case brSignal <- struct{}{}:
	default:
	}
}

func brightnessWorker() {
	for range brSignal {
		func() {
			defer recoverLog("brightness")
			d := int(brDelta.Swap(0))
			cyc := brCycleF.Swap(false)
			applyBrightness(d, cyc)
		}()
	}
}

func enumDDC() {
	for _, m := range ddcMons {
		procDestroyPhysicalMonitor.Call(m.h)
	}
	ddcMons = nil
	hmonitors = nil
	if procEnumDisplayMonitors.Find() != nil || procGetVCPFeatureAndVCPFeatureReply.Find() != nil {
		return
	}
	procEnumDisplayMonitors.Call(0, 0, monEnumCB, 0)
	for _, hm := range hmonitors {
		var n uint32
		r, _, _ := procGetNumberOfPhysicalMonitorsFromHMONITOR.Call(hm, uintptr(unsafe.Pointer(&n)))
		if r == 0 || n == 0 {
			continue
		}
		arr := make([]physicalMonitor, n)
		r, _, _ = procGetPhysicalMonitorsFromHMONITOR.Call(hm, uintptr(n), uintptr(unsafe.Pointer(&arr[0])))
		if r == 0 {
			continue
		}
		for _, pm := range arr {
			var cur, max uint32
			ok, _, _ := procGetVCPFeatureAndVCPFeatureReply.Call(pm.Handle, 0x10, 0, uintptr(unsafe.Pointer(&cur)), uintptr(unsafe.Pointer(&max)))
			if ok != 0 && max > 0 {
				ddcMons = append(ddcMons, ddcMonitor{h: pm.Handle, max: max, name: syscall.UTF16ToString(pm.Description[:])})
			} else {
				procDestroyPhysicalMonitor.Call(pm.Handle)
			}
		}
	}
	ddcEnumAt = time.Now()
	logf("DDC/CI monitors with brightness control: %d", len(ddcMons))
}

func readCurrentBrightness() int {
	for _, m := range ddcMons {
		var cur, max uint32
		ok, _, _ := procGetVCPFeatureAndVCPFeatureReply.Call(m.h, 0x10, 0, uintptr(unsafe.Pointer(&cur)), uintptr(unsafe.Pointer(&max)))
		if ok != 0 && max > 0 {
			return int(cur * 100 / max)
		}
	}
	if wmiCount > 0 {
		if out, err := wmi.query("[Console]::Out.WriteLine('B=' + (@(Get-WmiObject -Namespace root/WMI -Class WmiMonitorBrightness -ErrorAction SilentlyContinue)[0].CurrentBrightness))", 5*time.Second); err == nil {
			for _, l := range out {
				if strings.HasPrefix(l, "B=") {
					if v, err := strconv.Atoi(strings.TrimSpace(l[2:])); err == nil {
						return v
					}
				}
			}
		}
	}
	return -1
}

func ensureDisplays() {
	if time.Since(ddcEnumAt) > 60*time.Second || len(ddcMons) == 0 && time.Since(ddcEnumAt) > 10*time.Second {
		enumDDC()
	}
	if !wmiChecked {
		wmiChecked = true
		wmi = startPS()
		if wmi != nil {
			out, err := wmi.query("$bm = @(Get-WmiObject -Namespace root/WMI -Class WmiMonitorBrightnessMethods -ErrorAction SilentlyContinue); [Console]::Out.WriteLine('N=' + $bm.Count)", 15*time.Second)
			if err == nil {
				for _, l := range out {
					if strings.HasPrefix(l, "N=") {
						wmiCount, _ = strconv.Atoi(strings.TrimSpace(l[2:]))
					}
				}
			} else {
				logf("WMI init: %v", err)
			}
			logf("WMI (built-in display) brightness controls: %d", wmiCount)
			if wmiCount == 0 {
				wmi.close()
				wmi = nil
			}
		}
	}
}

func applyBrightness(delta int, cycle bool) {
	brMu.Lock()
	defer brMu.Unlock()
	ensureDisplays()
	if len(ddcMons) == 0 && wmiCount == 0 {
		osdShow("Brightness: no supported display", -1, false)
		return
	}
	if brCurrent < 0 || time.Since(brLastOp) > 8*time.Second {
		brCurrent = readCurrentBrightness()
		if brCurrent < 0 {
			brCurrent = 50
		}
	}
	target := brCurrent
	if cycle {
		target = brPresets[0]
		for _, p := range brPresets {
			if p > brCurrent {
				target = p
				break
			}
		}
	} else {
		target = clampPct(brCurrent + delta)
	}
	okAny := false
	for _, m := range ddcMons {
		v := uint32(target) * m.max / 100
		r, _, _ := procSetVCPFeature.Call(m.h, 0x10, uintptr(v))
		if r != 0 {
			okAny = true
		}
	}
	if wmiCount > 0 && wmi != nil {
		if err := wmi.send(fmt.Sprintf("foreach($m in $bm){ [void]$m.WmiSetBrightness(0, %d) }", target)); err == nil {
			okAny = true
		}
	}
	if !okAny && len(ddcMons) > 0 {
		ddcEnumAt = time.Time{} // force re-enumeration next time
	}
	brCurrent = target
	brLastOp = time.Now()
	osdShow("Brightness", target, false)
}

func brightnessInfo() map[string]interface{} {
	brMu.Lock()
	defer brMu.Unlock()
	var names []string
	for _, m := range ddcMons {
		names = append(names, m.name)
	}
	return map[string]interface{}{"ddc": len(ddcMons), "ddcNames": names, "builtin": wmiCount, "checked": wmiChecked}
}

// ---------- persistent hidden PowerShell ----------

type psHost struct {
	cmd   *exec.Cmd
	in    io.WriteCloser
	lines chan string
	mu    sync.Mutex
}

func startPS() *psHost {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NoLogo", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "-")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil
	}
	if err := cmd.Start(); err != nil {
		logf("powershell start: %v", err)
		return nil
	}
	p := &psHost{cmd: cmd, in: in, lines: make(chan string, 64)}
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			select {
			case p.lines <- sc.Text():
			default:
			}
		}
		close(p.lines)
	}()
	return p
}

func (p *psHost) send(line string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := io.WriteString(p.in, line+"\r\n")
	return err
}

func (p *psHost) query(line string, timeout time.Duration) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// drain stale output
	for {
		select {
		case <-p.lines:
			continue
		default:
		}
		break
	}
	if _, err := io.WriteString(p.in, line+"; [Console]::Out.WriteLine('__END__'); [Console]::Out.Flush()\r\n"); err != nil {
		return nil, err
	}
	var out []string
	t := time.After(timeout)
	for {
		select {
		case l, ok := <-p.lines:
			if !ok {
				return out, fmt.Errorf("powershell exited")
			}
			if strings.TrimSpace(l) == "__END__" {
				return out, nil
			}
			out = append(out, l)
		case <-t:
			return out, fmt.Errorf("powershell timeout")
		}
	}
}

func (p *psHost) close() {
	p.in.Close()
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
}
