package main

import (
	"fmt"
	"strings"
)

const (
	modCtrl  = 1
	modShift = 2
	modAlt   = 4
	modWin   = 8
)

const (
	vkShift    = 0x10
	vkControl  = 0x11
	vkMenu     = 0x12
	vkLWin     = 0x5B
	vkRWin     = 0x5C
	vkLShift   = 0xA0
	vkRShift   = 0xA1
	vkLControl = 0xA2
	vkRControl = 0xA3
	vkLMenu    = 0xA4
	vkRMenu    = 0xA5
	vkReturn   = 0x0D
)

var keyNames = map[string]uint16{}    // lower-case name -> vk
var vkCanonical = map[uint16]string{} // vk -> display name

func init() {
	add := func(vk uint16, canon string, aliases ...string) {
		keyNames[strings.ToLower(canon)] = vk
		for _, a := range aliases {
			keyNames[strings.ToLower(a)] = vk
		}
		if _, ok := vkCanonical[vk]; !ok {
			vkCanonical[vk] = canon
		}
	}
	for c := 'A'; c <= 'Z'; c++ {
		add(uint16(c), string(c))
	}
	for c := '0'; c <= '9'; c++ {
		add(uint16(c), string(c))
	}
	for i := 1; i <= 24; i++ {
		add(uint16(0x6F+i), fmt.Sprintf("F%d", i))
	}
	for i := 0; i <= 9; i++ {
		add(uint16(0x60+i), fmt.Sprintf("Num%d", i), fmt.Sprintf("Numpad%d", i))
	}
	add(vkControl, "Ctrl", "Control")
	add(vkShift, "Shift")
	add(vkMenu, "Alt")
	add(vkLWin, "Win", "Windows", "Meta", "Super", "Cmd")
	add(0x0D, "Enter", "Return")
	add(0x1B, "Esc", "Escape")
	add(0x09, "Tab")
	add(0x20, "Space")
	add(0x08, "Backspace")
	add(0x2E, "Delete", "Del")
	add(0x2D, "Insert", "Ins")
	add(0x24, "Home")
	add(0x23, "End")
	add(0x21, "PageUp", "PgUp")
	add(0x22, "PageDown", "PgDn")
	add(0x26, "Up")
	add(0x28, "Down")
	add(0x25, "Left")
	add(0x27, "Right")
	add(0x2C, "PrintScreen", "PrtSc", "Print")
	add(0x14, "CapsLock")
	add(0x90, "NumLock")
	add(0x91, "ScrollLock")
	add(0x13, "Pause")
	add(0x5D, "Apps", "Menu")
	add(0x6A, "NumMultiply")
	add(0x6B, "NumAdd")
	add(0x6D, "NumSubtract")
	add(0x6E, "NumDecimal")
	add(0x6F, "NumDivide")
	add(0xBB, "Plus", "=", "+", "Equals")
	add(0xBD, "Minus", "-")
	add(0xBC, "Comma", ",")
	add(0xBE, "Period", ".")
	add(0xAF, "VolumeUp")
	add(0xAE, "VolumeDown")
	add(0xAD, "VolumeMute")
	add(0xB3, "PlayPause")
	add(0xB0, "NextTrack")
	add(0xB1, "PrevTrack")
	add(0xB2, "MediaStop")
	add(0xA6, "BrowserBack")
	add(0xA7, "BrowserForward")
	add(0xAC, "BrowserHome")
	// generic OEM keys for learned codes
	add(0xBA, "OEM_1")
	add(0xBF, "OEM_2")
	add(0xC0, "OEM_3")
	add(0xDB, "OEM_4")
	add(0xDC, "OEM_5")
	add(0xDD, "OEM_6")
	add(0xDE, "OEM_7")
	add(0xE2, "OEM_102")
}

func isModifierVK(vk uint32) bool {
	switch vk {
	case vkShift, vkControl, vkMenu, vkLWin, vkRWin, vkLShift, vkRShift, vkLControl, vkRControl, vkLMenu, vkRMenu:
		return true
	}
	return false
}

func vkName(vk uint16) string {
	if n, ok := vkCanonical[vk]; ok {
		return n
	}
	return fmt.Sprintf("VK_%02X", vk)
}

func modsString(m uint32) string {
	var p []string
	if m&modCtrl != 0 {
		p = append(p, "Ctrl")
	}
	if m&modShift != 0 {
		p = append(p, "Shift")
	}
	if m&modAlt != 0 {
		p = append(p, "Alt")
	}
	if m&modWin != 0 {
		p = append(p, "Win")
	}
	return strings.Join(p, "+")
}

// trigger code = vk | mods<<16
func codeString(code uint32) string {
	s := vkName(uint16(code & 0xFFFF))
	if m := modsString(code >> 16); m != "" {
		return m + "+" + s
	}
	return s
}

func lookupKey(name string) (uint16, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if vk, ok := keyNames[n]; ok {
		return vk, true
	}
	if strings.HasPrefix(n, "vk_") && len(n) == 5 {
		var v uint16
		if _, err := fmt.Sscanf(n[3:], "%02x", &v); err == nil {
			return v, true
		}
	}
	return 0, false
}

// splitKeys splits "Ctrl+Shift+A" into parts; a trailing "++" means the Plus key.
func splitKeys(s string) []string {
	s = strings.TrimSpace(s)
	if s == "+" {
		return []string{"Plus"}
	}
	if strings.HasSuffix(s, "++") {
		s = s[:len(s)-1] + "Plus"
	}
	var out []string
	for _, p := range strings.Split(s, "+") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseTrigger(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	var mods uint32
	var key uint16
	for _, p := range splitKeys(s) {
		vk, ok := lookupKey(p)
		if !ok {
			return 0, fmt.Errorf("unknown key %q", p)
		}
		switch vk {
		case vkControl:
			mods |= modCtrl
			continue
		case vkShift:
			mods |= modShift
			continue
		case vkMenu:
			mods |= modAlt
			continue
		case vkLWin:
			mods |= modWin
			continue
		}
		if key != 0 {
			return 0, fmt.Errorf("only one non-modifier key allowed")
		}
		key = vk
	}
	if key == 0 {
		return 0, fmt.Errorf("needs a non-modifier key")
	}
	return uint32(key) | mods<<16, nil
}

// parseShortcut returns the VK sequence (modifiers first) for SendInput.
func parseShortcut(s string) ([]uint16, error) {
	var out []uint16
	for _, p := range splitKeys(s) {
		vk, ok := lookupKey(p)
		if !ok {
			return nil, fmt.Errorf("unknown key %q", p)
		}
		out = append(out, vk)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty shortcut")
	}
	return out, nil
}

func isExtendedVK(vk uint16) bool {
	switch {
	case vk >= 0x21 && vk <= 0x28, vk == 0x2D, vk == 0x2E, vk == 0x2C:
		return true
	case vk == vkLWin, vk == vkRWin, vk == 0x5D, vk == 0x6F, vk == 0x90:
		return true
	case vk == vkRControl, vk == vkRMenu:
		return true
	case vk >= 0xA6 && vk <= 0xB7:
		return true
	}
	return false
}

// vkToHID maps a Windows VK to a USB HID keyboard usage (for programming the pad).
func vkToHID(vk uint16) (byte, bool) {
	switch {
	case vk >= 'A' && vk <= 'Z':
		return byte(0x04 + vk - 'A'), true
	case vk >= '1' && vk <= '9':
		return byte(0x1E + vk - '1'), true
	case vk == '0':
		return 0x27, true
	case vk >= 0x70 && vk <= 0x7B: // F1-F12
		return byte(0x3A + vk - 0x70), true
	case vk >= 0x7C && vk <= 0x87: // F13-F24
		return byte(0x68 + vk - 0x7C), true
	case vk >= 0x61 && vk <= 0x69: // Num1-9
		return byte(0x59 + vk - 0x61), true
	}
	m := map[uint16]byte{
		0x0D: 0x28, 0x1B: 0x29, 0x08: 0x2A, 0x09: 0x2B, 0x20: 0x2C, 0xBD: 0x2D, 0xBB: 0x2E,
		0xDB: 0x2F, 0xDD: 0x30, 0xDC: 0x31, 0xBA: 0x33, 0xDE: 0x34, 0xC0: 0x35, 0xBC: 0x36,
		0xBE: 0x37, 0xBF: 0x38, 0x14: 0x39, 0x2C: 0x46, 0x91: 0x47, 0x13: 0x48, 0x2D: 0x49,
		0x24: 0x4A, 0x21: 0x4B, 0x2E: 0x4C, 0x23: 0x4D, 0x22: 0x4E, 0x27: 0x4F, 0x25: 0x50,
		0x28: 0x51, 0x26: 0x52, 0x90: 0x53, 0x6F: 0x54, 0x6A: 0x55, 0x6D: 0x56, 0x6B: 0x57,
		0x60: 0x62, 0x6E: 0x63, 0x5D: 0x65, 0xE2: 0x64,
	}
	h, ok := m[vk]
	return h, ok
}

func modsToHID(m uint32) byte {
	var b byte
	if m&modCtrl != 0 {
		b |= 0x01
	}
	if m&modShift != 0 {
		b |= 0x02
	}
	if m&modAlt != 0 {
		b |= 0x04
	}
	if m&modWin != 0 {
		b |= 0x08
	}
	return b
}

// Only F13–F24 (optionally with Shift and/or Ctrl) are allowed as keypad triggers,
// so the keypad never collides with keys on the normal keyboard.
var triggerModSets = []uint32{0, modShift, modCtrl, modCtrl | modShift}

func allowedTrigger(code uint32) bool {
	vk := code & 0xFFFF
	if vk < 0x7C || vk > 0x87 {
		return false
	}
	m := code >> 16
	for _, a := range triggerModSets {
		if m == a {
			return true
		}
	}
	return false
}

func triggerChoices() []string {
	var out []string
	for _, m := range triggerModSets {
		for f := 13; f <= 24; f++ {
			out = append(out, codeString(uint32(0x7C+f-13)|m<<16))
		}
	}
	return out
}

// triggerIndex is the combo index (0 = none) for a trigger string.
func triggerIndex(t string) int {
	for i, c := range triggerChoices() {
		if c == t {
			return i + 1
		}
	}
	return 0
}
