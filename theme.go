//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

type Theme struct {
	ID, Name                           string
	Dark                               bool
	Bg, Panel, Border, Text, Muted     uintptr
	Accent, AccentText, Key, KeyBorder uintptr
	Flash, Input, InputText, Dial      uintptr
	Good, Warn, Bad                    uintptr
}

var themes = []Theme{
	{ID: "light", Name: "Light", Bg: rgb(243, 244, 247), Panel: rgb(255, 255, 255), Border: rgb(222, 225, 231), Text: rgb(30, 32, 38), Muted: rgb(110, 116, 128),
		Accent: rgb(37, 99, 235), AccentText: rgb(255, 255, 255), Key: rgb(243, 244, 247), KeyBorder: rgb(200, 204, 212), Flash: rgb(191, 219, 254),
		Input: rgb(255, 255, 255), InputText: rgb(30, 32, 38), Dial: rgb(48, 53, 63), Good: rgb(22, 128, 61), Warn: rgb(180, 105, 0), Bad: rgb(200, 40, 40)},
	{ID: "dark", Name: "Dark", Dark: true, Bg: rgb(17, 19, 24), Panel: rgb(27, 30, 37), Border: rgb(44, 49, 60), Text: rgb(232, 234, 239), Muted: rgb(139, 147, 163),
		Accent: rgb(90, 156, 255), AccentText: rgb(255, 255, 255), Key: rgb(39, 44, 55), KeyBorder: rgb(58, 65, 80), Flash: rgb(46, 90, 170),
		Input: rgb(33, 37, 46), InputText: rgb(232, 234, 239), Dial: rgb(14, 16, 21), Good: rgb(62, 207, 142), Warn: rgb(245, 184, 75), Bad: rgb(255, 107, 107)},
	{ID: "midnight", Name: "Midnight blue", Dark: true, Bg: rgb(11, 18, 38), Panel: rgb(18, 28, 56), Border: rgb(36, 52, 94), Text: rgb(226, 234, 255), Muted: rgb(140, 156, 196),
		Accent: rgb(56, 189, 248), AccentText: rgb(8, 20, 40), Key: rgb(28, 42, 80), KeyBorder: rgb(48, 68, 120), Flash: rgb(14, 116, 144),
		Input: rgb(22, 34, 66), InputText: rgb(226, 234, 255), Dial: rgb(8, 13, 28), Good: rgb(74, 222, 128), Warn: rgb(250, 204, 21), Bad: rgb(248, 113, 113)},
	{ID: "forest", Name: "Forest", Dark: true, Bg: rgb(16, 24, 20), Panel: rgb(24, 36, 30), Border: rgb(40, 60, 50), Text: rgb(226, 240, 230), Muted: rgb(140, 170, 150),
		Accent: rgb(132, 204, 22), AccentText: rgb(16, 24, 20), Key: rgb(34, 52, 43), KeyBorder: rgb(54, 80, 66), Flash: rgb(63, 98, 18),
		Input: rgb(30, 46, 38), InputText: rgb(226, 240, 230), Dial: rgb(10, 16, 13), Good: rgb(132, 204, 22), Warn: rgb(250, 204, 21), Bad: rgb(248, 113, 113)},
	{ID: "lavender", Name: "Lavender", Dark: true, Bg: rgb(22, 16, 34), Panel: rgb(33, 24, 50), Border: rgb(58, 44, 86), Text: rgb(238, 230, 255), Muted: rgb(165, 150, 200),
		Accent: rgb(232, 121, 249), AccentText: rgb(30, 10, 40), Key: rgb(48, 36, 72), KeyBorder: rgb(76, 58, 112), Flash: rgb(120, 50, 140),
		Input: rgb(42, 31, 64), InputText: rgb(238, 230, 255), Dial: rgb(14, 10, 22), Good: rgb(74, 222, 128), Warn: rgb(250, 204, 21), Bad: rgb(251, 113, 133)},
	{ID: "sunset", Name: "Sunset", Bg: rgb(253, 246, 238), Panel: rgb(255, 252, 248), Border: rgb(240, 222, 204), Text: rgb(60, 36, 22), Muted: rgb(150, 112, 88),
		Accent: rgb(234, 88, 12), AccentText: rgb(255, 255, 255), Key: rgb(252, 236, 220), KeyBorder: rgb(236, 200, 170), Flash: rgb(254, 215, 170),
		Input: rgb(255, 255, 255), InputText: rgb(60, 36, 22), Dial: rgb(90, 50, 30), Good: rgb(22, 128, 61), Warn: rgb(180, 90, 0), Bad: rgb(200, 40, 40)},
	{ID: "mint", Name: "Mint", Bg: rgb(238, 248, 244), Panel: rgb(250, 255, 253), Border: rgb(204, 232, 220), Text: rgb(20, 48, 40), Muted: rgb(96, 130, 118),
		Accent: rgb(13, 148, 136), AccentText: rgb(255, 255, 255), Key: rgb(226, 244, 237), KeyBorder: rgb(176, 216, 200), Flash: rgb(153, 246, 228),
		Input: rgb(255, 255, 255), InputText: rgb(20, 48, 40), Dial: rgb(22, 60, 52), Good: rgb(21, 128, 61), Warn: rgb(180, 105, 0), Bad: rgb(200, 40, 40)},
}

var (
	th                              = themes[0]
	bgBrush, panelBrush, inputBrush uintptr
	modUxtheme                      = syscall.NewLazyDLL("uxtheme.dll")
	modDwmapi                       = syscall.NewLazyDLL("dwmapi.dll")
	procSetWindowTheme              = modUxtheme.NewProc("SetWindowTheme")
	procDwmSetWindowAttribute       = modDwmapi.NewProc("DwmSetWindowAttribute")
)

// semantic text roles for static controls
const (
	colText = iota
	colMuted
	colGood
	colWarn
	colBad
	colAccent
)

func themeColor(role int) uintptr {
	switch role {
	case colMuted:
		return th.Muted
	case colGood:
		return th.Good
	case colWarn:
		return th.Warn
	case colBad:
		return th.Bad
	case colAccent:
		return th.Accent
	}
	return th.Text
}

func themeByID(id string) Theme {
	for _, t := range themes {
		if t.ID == id {
			return t
		}
	}
	return themes[0]
}

func themeIndex(id string) int {
	for i, t := range themes {
		if t.ID == id {
			return i
		}
	}
	return 0
}

func makeBrushes() {
	for _, b := range []uintptr{bgBrush, panelBrush, inputBrush} {
		if b != 0 {
			procDeleteObject.Call(b)
		}
	}
	bgBrush, _, _ = procCreateSolidBrush.Call(th.Bg)
	panelBrush, _, _ = procCreateSolidBrush.Call(th.Panel)
	inputBrush, _, _ = procCreateSolidBrush.Call(th.Input)
}

// themeControl gives standard controls a dark or light look to match the theme.
func themeControl(h uintptr, multiline bool) {
	if procSetWindowTheme.Find() != nil || h == 0 {
		return
	}
	if th.Dark {
		name := "DarkMode_CFD"
		if multiline {
			name = "DarkMode_Explorer"
		}
		procSetWindowTheme.Call(h, u16ptr(name), 0)
	} else {
		procSetWindowTheme.Call(h, u16ptr("Explorer"), 0)
	}
}

func applyTheme(id string) {
	th = themeByID(id)
	makeBrushes()
	if settingsHwnd == 0 {
		return
	}
	if procDwmSetWindowAttribute.Find() == nil {
		var on int32
		if th.Dark {
			on = 1
		}
		r, _, _ := procDwmSetWindowAttribute.Call(settingsHwnd, 20, uintptr(unsafe.Pointer(&on)), 4)
		if r != 0 {
			procDwmSetWindowAttribute.Call(settingsHwnd, 19, uintptr(unsafe.Pointer(&on)), 4)
		}
	}
	for _, h := range []uintptr{edLabel, edP1, edP2, edRows, edCols, edKnobs, edVStep, edBStep, edAStep,
		cbMode, cbAction, cbTrig, cbProto, cbTheme, cbRotation} {
		themeControl(h, false)
	}
	themeControl(edText, true)
	themeControl(flashLog, true)
	procRedrawWindow.Call(settingsHwnd, 0, 0, 0x1|0x4|0x80|0x400) // INVALIDATE|ERASE|ALLCHILDREN|FRAME
	// nudge the title bar to repaint with the new mode
	procSetWindowPos.Call(settingsHwnd, 0, 0, 0, 0, 0, 0x1|0x2|0x4|0x10|0x20) // NOSIZE|NOMOVE|NOZORDER|NOACTIVATE|FRAMECHANGED
}
