package main

import (
	"bytes"
	"testing"
)

func TestProtocolFor(t *testing.T) {
	cases := []struct {
		vid, pid uint16
		pref     string
		want     string
	}{
		{0x514C, 0x8850, "auto", "8850"},
		{0x1189, 0x8890, "auto", "8890"},
		{0x1189, 0x8840, "auto", "884x"},
		{0x514C, 0x8850, "884x", "884x"},
	}
	for _, c := range cases {
		if got := protocolFor(c.vid, c.pid, c.pref); got != c.want {
			t.Errorf("protocolFor(%04x,%04x,%q) = %q, want %q", c.vid, c.pid, c.pref, got, c.want)
		}
	}
}

func TestFlash8850Packets(t *testing.T) {
	// Shift+F14 on knob 2 / turn left (key id 20)
	msgs := flash8850([]flashItem{{slot: "n2_ccw", id: 20, mods: 0x02, code: 0x69}})
	if len(msgs) != 2 {
		t.Fatalf("want binding + commit, got %d packets", len(msgs))
	}
	want := []byte{0x03, 0xFD, 20, 0x01, 0x01, 0x00, 2, 0, 0, 0xF2, 0, 0, 0x69}
	if !bytes.Equal(msgs[0].data, want) {
		t.Errorf("binding packet\n got %s\nwant %s", hexs(msgs[0].data), hexs(want))
	}
	if !bytes.Equal(msgs[1].data, []byte{0x03, 0xFD, 0xFE, 0xFF}) {
		t.Errorf("commit packet = %s", hexs(msgs[1].data))
	}
}
