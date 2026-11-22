//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

type Action struct {
	Type string `json:"type"`
	Keys string `json:"keys,omitempty"`
	Text string `json:"text,omitempty"`
	Path string `json:"path,omitempty"`
	Args string `json:"args,omitempty"`
}

type Slot struct {
	Label   string `json:"label"`
	Trigger string `json:"trigger"`
	Action  Action `json:"action"`
}

type Config struct {
	Rows           int              `json:"rows"`
	Cols           int              `json:"cols"`
	Knobs          int              `json:"knobs"`
	VolumeStep     int              `json:"volumeStep"`
	BrightnessStep int              `json:"brightnessStep"`
	AppVolumeStep  int              `json:"appVolumeStep"`
	ShowOSD        bool             `json:"showOsd"`
	Protocol       string           `json:"protocol"` // auto | 884x | 8890
	Theme          string           `json:"theme"`
	Rotation       int              `json:"rotation"` // 0/90/180/270: how the pad numbers its keys
	Version        int              `json:"version"`
	Slots          map[string]*Slot `json:"slots"`
}

const maxKeys = 16
const maxKnobs = 3

var knobParts = []string{"ccw", "press", "cw"}

var (
	cfgMu   sync.RWMutex
	cfg     *Config
	appDir  string
	cfgPath string
)

func keySlotID(i int) string        { return fmt.Sprintf("k%d", i+1) }
func knobSlotID(k, part int) string { return fmt.Sprintf("n%d_%s", k+1, knobParts[part]) }

// Default trigger codes: keys that no normal keyboard has (F13-F24).
// Knobs (used most, often fast) get plain F13-F21 so no modifier taps reach other apps.
// Keys: F22-F24, then Shift+F13..F24, then Ctrl+F13.
func defaultTrigger(id string) string {
	for k := 0; k < maxKnobs; k++ {
		for p := 0; p < 3; p++ {
			if id == knobSlotID(k, p) {
				return fmt.Sprintf("F%d", 13+k*3+p)
			}
		}
	}
	for i := 0; i < maxKeys; i++ {
		if id == keySlotID(i) {
			switch {
			case i < 3:
				return fmt.Sprintf("F%d", 22+i)
			case i < 15:
				return fmt.Sprintf("Shift+F%d", 13+i-3)
			default:
				return "Ctrl+F13"
			}
		}
	}
	return ""
}

func defaultConfig() *Config {
	c := &Config{Version: configVersion, Rows: 4, Cols: 4, Knobs: 3, Rotation: 90, VolumeStep: 2, BrightnessStep: 5, AppVolumeStep: 5, ShowOSD: true, Protocol: "auto", Theme: "dark", Slots: map[string]*Slot{}}
	keys := []Slot{
		{"Play / Pause", "", Action{Type: "media_playpause"}},
		{"Previous", "", Action{Type: "media_prev"}},
		{"Next", "", Action{Type: "media_next"}},
		{"Mic mute", "", Action{Type: "mic_mute"}},
		{"Screenshot", "", Action{Type: "shortcut", Keys: "Win+Shift+S"}},
		{"Task Manager", "", Action{Type: "shortcut", Keys: "Ctrl+Shift+Esc"}},
		{"Explorer", "", Action{Type: "shortcut", Keys: "Win+E"}},
		{"Show desktop", "", Action{Type: "shortcut", Keys: "Win+D"}},
		{"Calculator", "", Action{Type: "launch", Path: "calc.exe"}},
		{"Copy", "", Action{Type: "shortcut", Keys: "Ctrl+C"}},
		{"Paste", "", Action{Type: "shortcut", Keys: "Ctrl+V"}},
		{"Lock PC", "", Action{Type: "lock"}},
		{"Undo", "", Action{Type: "shortcut", Keys: "Ctrl+Z"}},
		{"Clipboard history", "", Action{Type: "shortcut", Keys: "Win+V"}},
		{"Task view", "", Action{Type: "shortcut", Keys: "Win+Tab"}},
		{"Settings", "", Action{Type: "launch", Path: "ms-settings:"}},
	}
	for i := 0; i < maxKeys; i++ {
		s := keys[i]
		s.Trigger = defaultTrigger(keySlotID(i))
		c.Slots[keySlotID(i)] = &s
	}
	knobs := [][3]Slot{
		{{"Volume down", "", Action{Type: "volume_down"}}, {"Mute", "", Action{Type: "volume_mute"}}, {"Volume up", "", Action{Type: "volume_up"}}},
		{{"Brightness down", "", Action{Type: "brightness_down"}}, {"Brightness presets", "", Action{Type: "brightness_cycle"}}, {"Brightness up", "", Action{Type: "brightness_up"}}},
		{{"App volume down", "", Action{Type: "appvol_down"}}, {"Mute app", "", Action{Type: "appvol_mute"}}, {"App volume up", "", Action{Type: "appvol_up"}}},
	}
	for k := 0; k < maxKnobs; k++ {
		for p := 0; p < 3; p++ {
			s := knobs[k][p]
			s.Trigger = defaultTrigger(knobSlotID(k, p))
			c.Slots[knobSlotID(k, p)] = &s
		}
	}
	return c
}

func allSlotIDs() []string {
	var ids []string
	for i := 0; i < maxKeys; i++ {
		ids = append(ids, keySlotID(i))
	}
	for k := 0; k < maxKnobs; k++ {
		for p := 0; p < 3; p++ {
			ids = append(ids, knobSlotID(k, p))
		}
	}
	return ids
}

const configVersion = 3

func normalize(c *Config) {
	d := defaultConfig()
	if c.Version < 2 {
		// default layout: 16-key / 3-knob pad (514C:8850) whose internal key order is rotated 90°
		c.Rows, c.Cols, c.Knobs, c.Rotation = 4, 4, 3, 90
		c.Version = configVersion
	}
	if c.Version < 3 {
		// 514C:8850 pads number their keys rotated 90° (verified from the factory letter layout)
		c.Rotation, c.Protocol = 90, "auto"
		c.Version = configVersion
	}
	if c.Rotation != 0 && c.Rotation != 90 && c.Rotation != 180 && c.Rotation != 270 {
		c.Rotation = 0
	}
	clamp := func(v *int, lo, hi, def int) {
		if *v < lo || *v > hi {
			*v = def
		}
	}
	clamp(&c.Rows, 1, 4, d.Rows)
	clamp(&c.Cols, 1, 6, d.Cols)
	clamp(&c.Knobs, 0, maxKnobs, d.Knobs)
	for c.Rows*c.Cols > maxKeys {
		c.Rows--
	}
	clamp(&c.VolumeStep, 1, 25, d.VolumeStep)
	clamp(&c.BrightnessStep, 1, 50, d.BrightnessStep)
	clamp(&c.AppVolumeStep, 1, 50, d.AppVolumeStep)
	if c.Protocol != "884x" && c.Protocol != "8890" && c.Protocol != "8850" {
		c.Protocol = "auto"
	}
	if c.Slots == nil {
		c.Slots = map[string]*Slot{}
	}
	for _, id := range allSlotIDs() {
		if c.Slots[id] == nil {
			c.Slots[id] = d.Slots[id]
		}
		if c.Slots[id].Action.Type == "" {
			c.Slots[id].Action.Type = "none"
		}
	}
	for id := range c.Slots {
		if d.Slots[id] == nil {
			delete(c.Slots, id)
		}
	}
	if themeByID(c.Theme).ID != c.Theme {
		c.Theme = d.Theme
	}
	// Triggers may only be F13–F24 (+Shift/Ctrl). Replace anything else (e.g. letters
	// learned from a factory-programmed pad) with the default code, then remove duplicates.
	seen := map[string]bool{}
	for _, id := range allSlotIDs() {
		s := c.Slots[id]
		if s.Trigger != "" {
			if code, err := parseTrigger(s.Trigger); err != nil || !allowedTrigger(code) {
				s.Trigger = defaultTrigger(id)
			} else {
				s.Trigger = codeString(code)
			}
		}
	}
	for _, id := range allSlotIDs() { // defaults win over custom codes when they collide
		if t := c.Slots[id].Trigger; t != "" && t == defaultTrigger(id) {
			seen[t] = true
		}
	}
	for _, id := range allSlotIDs() {
		s := c.Slots[id]
		if s.Trigger == "" || s.Trigger == defaultTrigger(id) {
			continue
		}
		if seen[s.Trigger] {
			s.Trigger = ""
		} else {
			seen[s.Trigger] = true
		}
	}
	// every slot must have a code, otherwise it isn't programmed and keeps its factory letter
	for _, id := range allSlotIDs() {
		if c.Slots[id].Trigger == "" {
			for _, t := range triggerChoices() {
				if !seen[t] {
					c.Slots[id].Trigger = t
					seen[t] = true
					break
				}
			}
		}
	}
}

// loadConfig returns true on first run (no config file yet).
func loadConfig() bool {
	cfgPath = filepath.Join(appDir, "config.json")
	c := defaultConfig()
	first := false
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		first = true
	} else if err := json.Unmarshal(b, c); err != nil {
		logf("config parse error, using defaults: %v", err)
		_ = os.WriteFile(cfgPath+".broken", b, 0644)
		c = defaultConfig()
	}
	normalize(c)
	cfgMu.Lock()
	cfg = c
	cfgMu.Unlock()
	rebuildBindings()
	if first {
		_ = saveConfig(c)
	}
	return first
}

func saveConfig(c *Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := cfgPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, cfgPath)
}

func setConfig(c *Config) error {
	normalize(c)
	cfgMu.Lock()
	cfg = c
	cfgMu.Unlock()
	rebuildBindings()
	return saveConfig(c)
}

func getConfigCopy() *Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	b, _ := json.Marshal(cfg)
	var c Config
	_ = json.Unmarshal(b, &c)
	return &c
}

// ---------- autostart (HKCU Run key) ----------

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runValue = "PadDeck"

func autostartEnabled() bool {
	var k syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, u16(runKey), 0, syscall.KEY_QUERY_VALUE, &k) != nil {
		return false
	}
	defer syscall.RegCloseKey(k)
	var typ, n uint32
	return syscall.RegQueryValueEx(k, u16(runValue), nil, &typ, nil, &n) == nil
}

func setAutostart(on bool) error {
	var k syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, u16(runKey), 0, syscall.KEY_SET_VALUE|syscall.KEY_QUERY_VALUE, &k); err != nil {
		return err
	}
	defer syscall.RegCloseKey(k)
	if !on {
		procRegDeleteValueW.Call(uintptr(k), u16ptr(runValue))
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	val, _ := syscall.UTF16FromString(`"` + exe + `" --background`)
	r, _, _ := procRegSetValueExW.Call(uintptr(k), u16ptr(runValue), 0, syscall.REG_SZ,
		uintptr(unsafe.Pointer(&val[0])), uintptr(len(val)*2))
	if r != 0 {
		return fmt.Errorf("RegSetValueEx failed: %d", r)
	}
	return nil
}
