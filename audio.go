//go:build windows

package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// Core Audio via raw COM vtables. All calls happen on the action worker thread (MTA).

const (
	eRender  = 0
	eCapture = 1
	eConsole = 0
	eComms   = 2
)

var (
	clsidMMDeviceEnumerator  = parseGUID("BCDE0395-E52F-467C-8E3D-C4579291692E")
	iidIMMDeviceEnumerator   = parseGUID("A95664D2-9614-4F35-A746-DE8DB63617E6")
	iidIAudioEndpointVolume  = parseGUID("5CDF2C82-841E-4546-9722-0CF74078229A")
	iidIAudioSessionManager2 = parseGUID("77AA99A0-1BD6-484F-8BC7-2C654C9A9B6F")
	iidIAudioSessionControl2 = parseGUID("BFB7FF88-7239-4FC9-8FA2-07C950BE9C6D")
	iidISimpleAudioVolume    = parseGUID("87CE5498-68D6-44E5-9215-6DA47EF883D8")

	deviceEnumerator uintptr
)

func getEnumerator() (uintptr, error) {
	if deviceEnumerator != 0 {
		return deviceEnumerator, nil
	}
	var e uintptr
	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&e)))
	if failed(hr) || e == 0 {
		return 0, fmt.Errorf("CoCreateInstance(MMDeviceEnumerator) hr=0x%08x", uint32(hr))
	}
	deviceEnumerator = e
	return e, nil
}

func defaultDevice(flow, role uint32) (uintptr, error) {
	e, err := getEnumerator()
	if err != nil {
		return 0, err
	}
	var dev uintptr
	hr, _, _ := syscall.SyscallN(vt(e, 4), e, uintptr(flow), uintptr(role), uintptr(unsafe.Pointer(&dev)))
	if failed(hr) || dev == 0 {
		return 0, fmt.Errorf("no default audio device (hr=0x%08x)", uint32(hr))
	}
	return dev, nil
}

func activate(dev uintptr, iid *guid) (uintptr, error) {
	var out uintptr
	hr, _, _ := syscall.SyscallN(vt(dev, 3), dev, uintptr(unsafe.Pointer(iid)), clsctxAll, 0, uintptr(unsafe.Pointer(&out)))
	if failed(hr) || out == 0 {
		return 0, fmt.Errorf("Activate hr=0x%08x", uint32(hr))
	}
	return out, nil
}

func endpointVolume(flow, role uint32) (uintptr, error) {
	dev, err := defaultDevice(flow, role)
	if err != nil {
		return 0, err
	}
	defer comRelease(dev)
	return activate(dev, &iidIAudioEndpointVolume)
}

func epGet(ep uintptr) (float32, bool) {
	var lvl float32
	var mute int32
	syscall.SyscallN(vt(ep, 9), ep, uintptr(unsafe.Pointer(&lvl)))
	syscall.SyscallN(vt(ep, 15), ep, uintptr(unsafe.Pointer(&mute)))
	return lvl, mute != 0
}

func epSetLevel(ep uintptr, v float32) {
	// Windows x64 ABI: Go's syscall stub mirrors integer args into XMM0-3, so a float
	// in the 2nd position arrives correctly in XMM1.
	syscall.SyscallN(vt(ep, 7), ep, uintptr(math.Float32bits(v)), 0)
}

func epSetMute(ep uintptr, m bool) {
	var b uintptr
	if m {
		b = 1
	}
	syscall.SyscallN(vt(ep, 14), ep, b, 0)
}

func pct(f float32) int { return int(math.Round(float64(f) * 100)) }

func clampPct(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// changeMasterVolume returns new percent and mute state.
func changeMasterVolume(flow uint32, delta int) (int, bool, error) {
	ep, err := endpointVolume(flow, eConsole)
	if err != nil {
		return 0, false, err
	}
	defer comRelease(ep)
	lvl, muted := epGet(ep)
	nv := clampPct(pct(lvl) + delta)
	// snap to the step grid for tidy numbers
	if delta != 0 {
		step := delta
		if step < 0 {
			step = -step
		}
		nv = clampPct((nv + step/2) / step * step)
		if delta > 0 && nv <= pct(lvl) {
			nv = clampPct(pct(lvl) + step)
		}
		if delta < 0 && nv >= pct(lvl) {
			nv = clampPct(pct(lvl) - step)
		}
	}
	epSetLevel(ep, float32(nv)/100)
	if muted && delta > 0 {
		epSetMute(ep, false)
		muted = false
	}
	return nv, muted, nil
}

func toggleMute(flow uint32) (int, bool, error) {
	ep, err := endpointVolume(flow, eConsole)
	if err != nil {
		return 0, false, err
	}
	lvl, muted := epGet(ep)
	epSetMute(ep, !muted)
	comRelease(ep)
	if flow == eCapture {
		// also apply to the default communications mic (Discord/Teams often use it)
		if ep2, err := endpointVolume(flow, eComms); err == nil {
			epSetMute(ep2, !muted)
			comRelease(ep2)
		}
	}
	return pct(lvl), !muted, nil
}

// ---------- per-application volume ----------

func exeOfPID(pid uint32) string {
	h, _, _ := procOpenProcess.Call(0x1000, 0, uintptr(pid)) // PROCESS_QUERY_LIMITED_INFORMATION
	if h == 0 {
		return ""
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	return strings.ToLower(filepath.Base(syscall.UTF16ToString(buf[:n])))
}

func pidOfWindow(hwnd uintptr) uint32 {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

var (
	childSearchHost uint32
	childFoundPID   uint32
	enumChildCB     = syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
		if p := pidOfWindow(hwnd); p != 0 && p != childSearchHost {
			childFoundPID = p
			return 0
		}
		return 1
	})
)

func foregroundExe() string {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return ""
	}
	pid := pidOfWindow(hwnd)
	exe := exeOfPID(pid)
	if exe == "applicationframehost.exe" { // UWP apps: find the real process
		childSearchHost, childFoundPID = pid, 0
		procEnumChildWindows.Call(hwnd, enumChildCB, 0)
		if childFoundPID != 0 {
			exe = exeOfPID(childFoundPID)
		}
	}
	return exe
}

func prettyExe(exe string) string {
	n := strings.TrimSuffix(exe, ".exe")
	if n == "" {
		return "App"
	}
	return strings.ToUpper(n[:1]) + n[1:]
}

// sessionsForExe returns ISimpleAudioVolume pointers (caller releases).
func sessionsForExe(exe string) ([]uintptr, error) {
	dev, err := defaultDevice(eRender, eConsole)
	if err != nil {
		return nil, err
	}
	defer comRelease(dev)
	mgr, err := activate(dev, &iidIAudioSessionManager2)
	if err != nil {
		return nil, err
	}
	defer comRelease(mgr)
	var en uintptr
	hr, _, _ := syscall.SyscallN(vt(mgr, 5), mgr, uintptr(unsafe.Pointer(&en)))
	if failed(hr) || en == 0 {
		return nil, fmt.Errorf("GetSessionEnumerator hr=0x%08x", uint32(hr))
	}
	defer comRelease(en)
	var n int32
	syscall.SyscallN(vt(en, 3), en, uintptr(unsafe.Pointer(&n)))
	var out []uintptr
	for i := int32(0); i < n; i++ {
		var ctl uintptr
		hr, _, _ := syscall.SyscallN(vt(en, 4), en, uintptr(i), uintptr(unsafe.Pointer(&ctl)))
		if failed(hr) || ctl == 0 {
			continue
		}
		var ctl2 uintptr
		syscall.SyscallN(vt(ctl, 0), ctl, uintptr(unsafe.Pointer(&iidIAudioSessionControl2)), uintptr(unsafe.Pointer(&ctl2)))
		if ctl2 != 0 {
			var pid uint32
			syscall.SyscallN(vt(ctl2, 14), ctl2, uintptr(unsafe.Pointer(&pid)))
			if pid != 0 && exeOfPID(pid) == exe {
				var sav uintptr
				syscall.SyscallN(vt(ctl, 0), ctl, uintptr(unsafe.Pointer(&iidISimpleAudioVolume)), uintptr(unsafe.Pointer(&sav)))
				if sav != 0 {
					out = append(out, sav)
				}
			}
			comRelease(ctl2)
		}
		comRelease(ctl)
	}
	return out, nil
}

func appVolumeAction(kind string, step int) {
	exe := foregroundExe()
	if exe == "" {
		return
	}
	name := prettyExe(exe)
	sess, err := sessionsForExe(exe)
	if err != nil || len(sess) == 0 {
		osdShow(name+": no audio", -1, false)
		return
	}
	defer func() {
		for _, s := range sess {
			comRelease(s)
		}
	}()
	var lvl float32
	var mute int32
	s0 := sess[0]
	syscall.SyscallN(vt(s0, 4), s0, uintptr(unsafe.Pointer(&lvl)))
	syscall.SyscallN(vt(s0, 6), s0, uintptr(unsafe.Pointer(&mute)))
	cur := pct(lvl)
	switch kind {
	case "appvol_mute":
		nm := uintptr(1)
		if mute != 0 {
			nm = 0
		}
		for _, s := range sess {
			syscall.SyscallN(vt(s, 5), s, nm, 0)
		}
		osdShow(name, cur, nm == 1)
		return
	case "appvol_up":
		cur = clampPct(cur + step)
	case "appvol_down":
		cur = clampPct(cur - step)
	}
	for _, s := range sess {
		syscall.SyscallN(vt(s, 3), s, uintptr(math.Float32bits(float32(cur)/100)), 0)
		if kind == "appvol_up" && mute != 0 {
			syscall.SyscallN(vt(s, 5), s, 0, 0)
		}
	}
	osdShow(name, cur, mute != 0 && kind != "appvol_up")
}
