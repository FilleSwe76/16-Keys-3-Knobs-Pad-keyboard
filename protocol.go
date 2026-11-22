package main

// Keypad programming protocols. Pure functions (no Windows calls) so they can be unit-tested.
// References (protocol research, no code copied):
//   - https://github.com/kriomant/ch57x-keyboard-tool  (1189:8840/8842/8890)
//   - https://github.com/elmitash/macropad-16key-3knob (514C:8850 "0xFD" variant)

import (
	"fmt"
	"strings"
	"time"
)

type flashItem struct {
	slot string
	id   byte
	mods byte
	code byte
}

func protocolFor(vid, pid uint16, pref string) string {
	if pref == "884x" || pref == "8890" || pref == "8850" {
		return pref
	}
	if vid == 0x514C && pid == 0x8850 {
		return "8850" // 0xFD protocol (16 keys + 3 knobs, verified layout)
	}
	if pid == 0x8890 {
		return "8890"
	}
	return "884x"
}

type flashMsg struct {
	data []byte
	wait time.Duration
}

// flash8850 builds the 514C:8850 "0xFD" protocol: one binding packet then a commit, per key.
// Binding: FD, key id, layer, kind(1=keyboard), 0, n codes, then n × [delay hi, delay lo, usage].
// Modifiers are codes of their own: F1 Ctrl, F2 Shift, F3 Alt, F4 Win.
func flash8850(items []flashItem) []flashMsg {
	var out []flashMsg
	for _, it := range items {
		var codes []byte
		for bit, mc := range []byte{0xF1, 0xF2, 0xF3, 0xF4} { // HID mod bits: 1 Ctrl, 2 Shift, 4 Alt, 8 Win
			if it.mods&(1<<bit) != 0 {
				codes = append(codes, mc)
			}
		}
		codes = append(codes, it.code)
		m := []byte{0x03, 0xFD, it.id, 0x01, 0x01, 0x00, byte(len(codes))}
		for _, c := range codes {
			m = append(m, 0x00, 0x00, c)
		}
		out = append(out,
			flashMsg{m, 300 * time.Millisecond},
			flashMsg{[]byte{0x03, 0xFD, 0xFE, 0xFF}, 800 * time.Millisecond})
	}
	return out
}

func flashMessages(proto string, items []flashItem) []flashMsg {
	if proto == "8850" {
		return flash8850(items)
	}
	var out []flashMsg
	for _, m := range flashMessagesLegacy(proto, items) {
		out = append(out, flashMsg{m, 25 * time.Millisecond})
	}
	return out
}

func flashMessagesLegacy(proto string, items []flashItem) [][]byte {
	var msgs [][]byte
	if proto == "8890" {
		for _, it := range items {
			msgs = append(msgs,
				[]byte{0x03, 0xa1, 0x01},
				[]byte{0x03, it.id, 0x10 | 0x01, 1, 0, it.mods, it.code},
				[]byte{0x03, 0xaa, 0xaa})
		}
		return msgs
	}
	for _, it := range items {
		msgs = append(msgs, []byte{0x03, 0xfe, it.id, 0x01, 0x01, 0, 0, 0, 0, 0, 1, it.mods, it.code})
	}
	msgs = append(msgs,
		[]byte{0x03, 0xaa, 0xaa},
		[]byte{0x03, 0xfd, 0xfe, 0xff},
		[]byte{0x03, 0xaa, 0xaa})
	return msgs
}

func hexs(b []byte) string {
	var p []string
	for _, x := range b {
		p = append(p, fmt.Sprintf("%02x", x))
	}
	return strings.Join(p, " ")
}
