package main

import "testing"

func TestParseTriggerRoundTrip(t *testing.T) {
	cases := map[string]string{
		"F13":            "F13",
		"shift+f14":      "Shift+F14",
		"Ctrl+Shift+F24": "Ctrl+Shift+F24",
		"ctrl++":         "Ctrl+Plus",
		"Win+Shift+S":    "Shift+Win+S",
	}
	for in, want := range cases {
		code, err := parseTrigger(in)
		if err != nil {
			t.Fatalf("parseTrigger(%q): %v", in, err)
		}
		if got := codeString(code); got != want {
			t.Errorf("parseTrigger(%q) -> %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "Ctrl", "Ctrl+Shift", "A+B", "Nope"} {
		if _, err := parseTrigger(bad); err == nil {
			t.Errorf("parseTrigger(%q) should fail", bad)
		}
	}
}

func TestTriggerChoicesAreAllowedAndUnique(t *testing.T) {
	ch := triggerChoices()
	if len(ch) != 48 {
		t.Fatalf("want 48 trigger choices, got %d", len(ch))
	}
	seen := map[string]bool{}
	for i, c := range ch {
		code, err := parseTrigger(c)
		if err != nil || !allowedTrigger(code) {
			t.Errorf("choice %q not allowed (%v)", c, err)
		}
		if seen[c] {
			t.Errorf("duplicate choice %q", c)
		}
		seen[c] = true
		if triggerIndex(c) != i+1 {
			t.Errorf("triggerIndex(%q) = %d, want %d", c, triggerIndex(c), i+1)
		}
	}
	for _, s := range []string{"A", "F12", "Alt+F13", "Win+F13", "Space"} {
		code, _ := parseTrigger(s)
		if allowedTrigger(code) {
			t.Errorf("%q must not be an allowed trigger", s)
		}
	}
}

func TestVKToHIDFunctionKeys(t *testing.T) {
	for i := 0; i < 12; i++ {
		h, ok := vkToHID(uint16(0x7C + i)) // VK_F13..VK_F24
		if !ok || h != byte(0x68+i) {     // HID F13..F24
			t.Errorf("F%d -> %02x ok=%v", 13+i, h, ok)
		}
	}
}

func TestParseShortcut(t *testing.T) {
	vks, err := parseShortcut("Ctrl+Shift+Esc")
	if err != nil || len(vks) != 3 || vks[0] != vkControl || vks[1] != vkShift || vks[2] != 0x1B {
		t.Fatalf("got %v %v", vks, err)
	}
	vks, err = parseShortcut("Ctrl++")
	if err != nil || len(vks) != 2 || vks[1] != 0xBB {
		t.Fatalf("Ctrl++: got %v %v", vks, err)
	}
}
