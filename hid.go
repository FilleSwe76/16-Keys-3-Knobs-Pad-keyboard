//go:build windows

package main

import (
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Programs CH57x-based macro pads (USB VID 0x1189: PIDs 8890, 8840, 8842 ...) so each key/knob
// sends PadDeck's unique trigger code. Protocol follows the open-source ch57x-keyboard-tool.

const ch57xVID = 0x1189

type hidDevice struct {
	Path      string `json:"-"`
	VID       uint16 `json:"vid"`
	PID       uint16 `json:"pid"`
	OutLen    uint16 `json:"outLen"`
	UsagePage uint16 `json:"usagePage"`
	Usage     uint16 `json:"usage"`
}

type spDeviceInterfaceData struct {
	CbSize             uint32
	InterfaceClassGuid guid
	Flags              uint32
	Reserved           uintptr
}

type hiddAttributes struct {
	Size          uint32
	VendorID      uint16
	ProductID     uint16
	VersionNumber uint16
	_             uint16
}

func enumCH57x() []hidDevice {
	var g guid
	procHidDGetHidGuid.Call(uintptr(unsafe.Pointer(&g)))
	set, _, _ := procSetupDiGetClassDevsW.Call(uintptr(unsafe.Pointer(&g)), 0, 0, 0x2|0x10) // PRESENT|DEVICEINTERFACE
	if set == 0 || set == ^uintptr(0) {
		return nil
	}
	defer procSetupDiDestroyDeviceInfoList.Call(set)
	var out []hidDevice
	for i := 0; i < 512; i++ {
		var ifd spDeviceInterfaceData
		ifd.CbSize = uint32(unsafe.Sizeof(ifd))
		r, _, _ := procSetupDiEnumDeviceInterfaces.Call(set, 0, uintptr(unsafe.Pointer(&g)), uintptr(i), uintptr(unsafe.Pointer(&ifd)))
		if r == 0 {
			break
		}
		var need uint32
		procSetupDiGetDeviceInterfaceDetailW.Call(set, uintptr(unsafe.Pointer(&ifd)), 0, 0, uintptr(unsafe.Pointer(&need)), 0)
		if need < 8 {
			continue
		}
		buf := make([]uint64, (need+7)/8)
		*(*uint32)(unsafe.Pointer(&buf[0])) = 8 // cbSize of SP_DEVICE_INTERFACE_DETAIL_DATA_W on x64
		r, _, _ = procSetupDiGetDeviceInterfaceDetailW.Call(set, uintptr(unsafe.Pointer(&ifd)), uintptr(unsafe.Pointer(&buf[0])), uintptr(need), 0, 0)
		if r == 0 {
			continue
		}
		nchars := (int(need) - 4) / 2
		path := syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0]))+4)), nchars))
		lp := strings.ToLower(path)
		if !strings.Contains(lp, "vid_1189") && !strings.Contains(lp, "vid_514c") {
			continue
		}
		h, err := syscall.CreateFile(u16(path), 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		attr := hiddAttributes{Size: uint32(unsafe.Sizeof(hiddAttributes{}))}
		procHidDGetAttributes.Call(uintptr(h), uintptr(unsafe.Pointer(&attr)))
		var pp uintptr
		var caps [64]uint16
		if r, _, _ := procHidDGetPreparsedData.Call(uintptr(h), uintptr(unsafe.Pointer(&pp))); r != 0 {
			procHidPGetCaps.Call(pp, uintptr(unsafe.Pointer(&caps[0])))
			procHidDFreePreparsedData.Call(pp)
		}
		syscall.CloseHandle(h)
		if attr.VendorID != ch57xVID && attr.VendorID != 0x514C {
			continue
		}
		out = append(out, hidDevice{Path: path, VID: attr.VendorID, PID: attr.ProductID, UsagePage: caps[1], Usage: caps[0], OutLen: caps[3]})
	}
	return out
}

// pickTarget chooses the vendor (configuration) interface of the pad.
func pickTarget(devs []hidDevice) *hidDevice {
	var best *hidDevice
	for i := range devs {
		d := &devs[i]
		if d.OutLen == 0 {
			continue
		}
		if best == nil || d.OutLen == 65 && best.OutLen != 65 || d.UsagePage >= 0xFF00 && best.UsagePage < 0xFF00 {
			best = d
		}
	}
	return best
}

// buildFlashPlan maps each configured slot to the pad's internal key id + HID code.
func buildFlashPlan(c *Config, proto string) ([]flashItem, []string) {
	var items []flashItem
	var warn []string
	base := 16
	if proto == "8890" {
		base = 12
	}
	nKeys := c.Rows * c.Cols
	if nKeys > base {
		warn = append(warn, fmt.Sprintf("this protocol supports at most %d keys; extra keys skipped", base))
		nKeys = base
	}
	add := func(slot string, id int) {
		s := c.Slots[slot]
		if s == nil || s.Trigger == "" {
			return
		}
		code, err := parseTrigger(s.Trigger)
		if err != nil {
			warn = append(warn, fmt.Sprintf("%s: bad trigger %q", slot, s.Trigger))
			return
		}
		hid, ok := vkToHID(uint16(code & 0xFFFF))
		if !ok {
			warn = append(warn, fmt.Sprintf("%s: %s can't be stored on the pad", slot, s.Trigger))
			return
		}
		items = append(items, flashItem{slot, byte(id), modsToHID(code >> 16), hid})
	}
	for i := 0; i < c.Rows*c.Cols; i++ {
		id := keyDeviceID(c.Rows, c.Cols, c.Rotation, i/c.Cols, i%c.Cols)
		if id > nKeys {
			continue
		}
		add(keySlotID(i), id)
	}
	for k := 0; k < c.Knobs; k++ {
		for p := 0; p < 3; p++ {
			add(knobSlotID(k, p), base+1+3*k+p)
		}
	}
	return items, warn
}

func flashPad(c *Config) (log []string, err error) {
	devs := enumCH57x()
	t := pickTarget(devs)
	if t == nil {
		return nil, fmt.Errorf("no supported keypad (USB ID 1189:xxxx or 514C:xxxx) found. Use your keypad's own software to assign the trigger codes shown in PadDeck, or use Learn")
	}
	for _, d := range devs {
		log = append(log, fmt.Sprintf("  interface: usage page %04X usage %04X, output report %d bytes", d.UsagePage, d.Usage, d.OutLen))
	}
	proto := protocolFor(t.VID, t.PID, c.Protocol)
	log = append(log, fmt.Sprintf("Keypad %04X:%04X, protocol %s, report size %d", t.VID, t.PID, proto, t.OutLen))
	items, warns := buildFlashPlan(c, proto)
	log = append(log, warns...)
	if len(items) == 0 {
		return log, fmt.Errorf("nothing to program")
	}
	h, e := syscall.CreateFile(u16(t.Path), syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	if e != nil {
		return log, fmt.Errorf("could not open keypad: %v (close the manufacturer's software and try again)", e)
	}
	defer syscall.CloseHandle(h)
	n := int(t.OutLen)
	write := func(m []byte) error {
		buf := make([]byte, n)
		copy(buf, m)
		var w uint32
		if err := syscall.WriteFile(h, buf, &w, nil); err != nil {
			r, _, _ := procHidDSetOutputReport.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(n))
			if r == 0 {
				return err
			}
		}
		return nil
	}
	for i, it := range items {
		log = append(log, fmt.Sprintf("%-9s → key id %2d  =  %s", it.slot, it.id, c.Slots[it.slot].Trigger))
		_ = i
	}
	for _, m := range flashMessages(proto, items) {
		if err := write(m.data); err != nil {
			log = append(log, "write failed: "+hexs(m.data))
			return log, fmt.Errorf("write to keypad failed: %v", err)
		}
		time.Sleep(m.wait)
	}
	log = append(log, "Done. Press a key on the pad: it should light up in PadDeck's keypad picture.",
		"If it still types letters, unplug the keypad, plug it back in and try once more.")
	return log, nil
}
