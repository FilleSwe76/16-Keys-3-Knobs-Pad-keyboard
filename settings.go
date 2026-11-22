//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// Native settings window (Win32 controls). Runs on the main GUI thread.

var (
	modComctl32 = syscall.NewLazyDLL("comctl32.dll")
	modComdlg32 = syscall.NewLazyDLL("comdlg32.dll")

	procSendMessageW         = modUser32.NewProc("SendMessageW")
	procGetWindowTextW       = modUser32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = modUser32.NewProc("GetWindowTextLengthW")
	procSetWindowTextW       = modUser32.NewProc("SetWindowTextW")
	procMoveWindow           = modUser32.NewProc("MoveWindow")
	procEnableWindow         = modUser32.NewProc("EnableWindow")
	procIsDialogMessageW     = modUser32.NewProc("IsDialogMessageW")
	procFindWindowW          = modUser32.NewProc("FindWindowW")
	procIsWindowVisible      = modUser32.NewProc("IsWindowVisible")
	procIsIconic             = modUser32.NewProc("IsIconic")
	procAdjustWindowRectEx   = modUser32.NewProc("AdjustWindowRectEx")
	procMessageBoxW          = modUser32.NewProc("MessageBoxW")
	procRedrawWindow         = modUser32.NewProc("RedrawWindow")
	procRoundRect            = modGdi32.NewProc("RoundRect")
	procEllipse              = modGdi32.NewProc("Ellipse")
	procCreatePen            = modGdi32.NewProc("CreatePen")
	procMoveToEx             = modGdi32.NewProc("MoveToEx")
	procLineTo               = modGdi32.NewProc("LineTo")
	procSetBkColor           = modGdi32.NewProc("SetBkColor")
	procGetClientRect        = modUser32.NewProc("GetClientRect")
	procCreateActCtxW        = modKernel32.NewProc("CreateActCtxW")
	procActivateActCtx       = modKernel32.NewProc("ActivateActCtx")
	procInitCommonControls   = modComctl32.NewProc("InitCommonControls")
	procGetOpenFileNameW     = modComdlg32.NewProc("GetOpenFileNameW")
)

const (
	wsChild           = 0x40000000
	wsVisible         = 0x10000000
	wsTabStop         = 0x00010000
	wsVScroll         = 0x00200000
	wsCaption         = 0x00C00000
	wsSysMenu         = 0x00080000
	wsMinimizeBox     = 0x00020000
	wsExClientEdge    = 0x00000200
	wsExControlParent = 0x00010000

	bsPushButton   = 0x0
	bsAutoCheckbox = 0x3
	bsGroupBox     = 0x7
	bsOwnerDraw    = 0xB

	esMultiline   = 0x4
	esAutoVScroll = 0x40
	esAutoHScroll = 0x80
	esReadOnly    = 0x800
	esWantReturn  = 0x1000
	esNumber      = 0x2000

	ssCenter      = 0x1
	ssNoPrefix    = 0x80
	ssEndEllipsis = 0x4000

	cbsDropDownList = 0x3

	wmSetFont        = 0x0030
	wmSetIcon        = 0x0080
	wmClose          = 0x0010
	wmCommand        = 0x0111
	wmDrawItem       = 0x002B
	wmCtlColorStatic = 0x0138
	wmCtlColorBtn    = 0x0135
	cbAddString      = 0x0143
	cbSetCurSel      = 0x014E
	cbGetCurSel      = 0x0147
	cbResetContent   = 0x014B
	bmGetCheck       = 0x00F0
	bmSetCheck       = 0x00F1
	emSetSel         = 0x00B1
	enChange         = 0x0300
	enKillFocus      = 0x0200
	cbnSelChange     = 1
	bnClicked        = 0

	wmAppShow  = wmApp + 10
	wmAppFired = wmApp + 11
	wmAppCall  = wmApp + 12

	timerSave  = 3
	timerFlash = 4
)

// control ids
const (
	idKeyBase  = 1000 // +i
	idDialBase = 1100 // +k
	idPartBase = 1200 // +k*3+p
	idMode     = 200
	idLabel    = 201
	idAction   = 202
	idP1       = 203
	idRecord   = 204
	idBrowse   = 205
	idText     = 206
	idP2       = 207
	idTrigger  = 208
	idLearn    = 209
	idTest     = 210
	idDetect   = 300
	idFlash    = 301
	idRows     = 400
	idCols     = 401
	idKnobs    = 402
	idVStep    = 403
	idBStep    = 404
	idAStep    = 405
	idProto    = 406
	idOSD      = 407
	idAuto     = 408
	idPause    = 409
	idDisplays = 410
	idReset    = 411
	idExit     = 412
	idTheme    = 413
	idRotation = 414
	idLayout   = 415
)

type actDef struct{ id, name string }

var actionDefs = []actDef{
	{"none", "Do nothing"},
	{"volume_up", "Sound: Volume up"},
	{"volume_down", "Sound: Volume down"},
	{"volume_mute", "Sound: Mute / unmute speakers"},
	{"appvol_up", "Sound: Focused app volume up"},
	{"appvol_down", "Sound: Focused app volume down"},
	{"appvol_mute", "Sound: Mute / unmute focused app"},
	{"mic_mute", "Sound: Microphone mute toggle"},
	{"brightness_up", "Display: Brightness up"},
	{"brightness_down", "Display: Brightness down"},
	{"brightness_cycle", "Display: Brightness presets (10/40/70/100%)"},
	{"media_playpause", "Media: Play / pause"},
	{"media_next", "Media: Next track"},
	{"media_prev", "Media: Previous track"},
	{"media_stop", "Media: Stop"},
	{"shortcut", "Keyboard: Shortcut"},
	{"text", "Keyboard: Type text / snippet"},
	{"launch", "Open: App, file, folder or website"},
	{"command", "Open: Run command (hidden)"},
	{"scroll_up", "Mouse: Scroll up"},
	{"scroll_down", "Mouse: Scroll down"},
	{"scroll_left", "Mouse: Scroll left"},
	{"scroll_right", "Mouse: Scroll right"},
	{"zoom_in", "Mouse: Zoom in (Ctrl+wheel)"},
	{"zoom_out", "Mouse: Zoom out (Ctrl+wheel)"},
	{"lock", "System: Lock PC"},
}

func actionShortName(id string) string {
	for _, a := range actionDefs {
		if a.id == id {
			if i := strings.Index(a.name, ": "); i >= 0 {
				return a.name[i+2:]
			}
			return a.name
		}
	}
	return ""
}

type knobPresetDef struct {
	id, name string
	acts     [3]actDef
}

var knobPresets = []knobPresetDef{
	{"volume", "Volume", [3]actDef{{"volume_down", "Volume down"}, {"volume_mute", "Mute"}, {"volume_up", "Volume up"}}},
	{"brightness", "Screen brightness", [3]actDef{{"brightness_down", "Brightness down"}, {"brightness_cycle", "Brightness presets"}, {"brightness_up", "Brightness up"}}},
	{"appvol", "Focused app volume", [3]actDef{{"appvol_down", "App volume down"}, {"appvol_mute", "Mute app"}, {"appvol_up", "App volume up"}}},
	{"tracks", "Media tracks", [3]actDef{{"media_prev", "Previous"}, {"media_playpause", "Play / pause"}, {"media_next", "Next"}}},
	{"scroll", "Scroll", [3]actDef{{"scroll_up", "Scroll up"}, {"none", "—"}, {"scroll_down", "Scroll down"}}},
	{"hscroll", "Horizontal scroll / timeline", [3]actDef{{"scroll_left", "Scroll left"}, {"none", "—"}, {"scroll_right", "Scroll right"}}},
	{"zoom", "Zoom", [3]actDef{{"zoom_out", "Zoom out"}, {"zoom_in", "Zoom in"}, {"zoom_in", "Zoom in"}}},
}

var partNames = [3]string{"Turn left", "Press", "Turn right"}

type drawItemStruct struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemAction uint32
	ItemState  uint32
	HwndItem   uintptr
	HDC        uintptr
	RcItem     rect
	ItemData   uintptr
}

type actCtx struct {
	CbSize                 uint32
	DwFlags                uint32
	LpSource               *uint16
	WProcessorArchitecture uint16
	WLangId                uint16
	LpAssemblyDirectory    *uint16
	LpResourceName         *uint16
	LpApplicationName      *uint16
	HModule                uintptr
}

type openFileName struct {
	LStructSize       uint32
	HwndOwner         uintptr
	HInstance         uintptr
	LpstrFilter       *uint16
	LpstrCustomFilter *uint16
	NMaxCustFilter    uint32
	NFilterIndex      uint32
	LpstrFile         *uint16
	NMaxFile          uint32
	LpstrFileTitle    *uint16
	NMaxFileTitle     uint32
	LpstrInitialDir   *uint16
	LpstrTitle        *uint16
	Flags             uint32
	NFileOffset       uint16
	NFileExtension    uint16
	LpstrDefExt       *uint16
	LCustData         uintptr
	LpfnHook          uintptr
	LpTemplateName    *uint16
	PvReserved        uintptr
	DwReserved        uint32
	FlagsEx           uint32
}

var (
	settingsHwnd uintptr
	settingsCB   = syscall.NewCallback(settingsProc)
	uiCalls      = make(chan func(), 64)

	uiCfg     *Config
	uiSel     = "n1_ccw"
	uiLoading bool
	flashSlot string

	fontNormal, fontSmall, fontBold, fontKey, fontMono, fontSym uintptr
	staticColor                                                 = map[uintptr]int{}
	pushBtns                                                    = map[uintptr]bool{}
	checkState                                                  = map[uintptr]bool{}
	primaryBtn                                                  uintptr
	cards                                                       []card

	keyBtns  [maxKeys]uintptr
	dialBtns [maxKnobs]uintptr
	partBtns [maxKnobs][3]uintptr
	knobLbls [maxKnobs]uintptr

	edHeader, lblMode, cbMode, lblLabel, edLabel, lblAction, cbAction    uintptr
	lblP1, edP1, btnRecord, btnBrowse, edText, lblP2, edP2, hintP        uintptr
	lblTrig, cbTrig, btnLearn, btnTest, learnMsg                         uintptr
	detStatus, flashLog, btnFlash                                        uintptr
	edRows, edCols, edKnobs, edVStep, edBStep, edAStep, cbProto, cbTheme uintptr
	chkOSD, chkAuto, chkPause, lastRx, dispRes, cbRotation, btnLayout    uintptr

	learnCancel chan struct{}
	learnMode   string // "trigger" | "shortcut"
)

func S(v float64) int32 { return int32(math.Round(v * dpiScale)) }

func sendMsg(h uintptr, m uint32, w, l uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(h, uintptr(m), w, l)
	return r
}

func getText(h uintptr) string {
	n, _, _ := procGetWindowTextLengthW.Call(h)
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func setText(h uintptr, s string) {
	if getText(h) != s {
		procSetWindowTextW.Call(h, u16ptr(s))
		if pushBtns[h] || checkState != nil && isCheck(h) {
			redraw(h)
		}
	}
}

func show(h uintptr, on bool) {
	if on {
		procShowWindow.Call(h, 5)
	} else {
		procShowWindow.Call(h, swHide)
	}
}

func move(h uintptr, x, y, w, ht int32) {
	procMoveWindow.Call(h, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), 1)
}

func setCheck(h uintptr, on bool) {
	if checkState[h] != on {
		checkState[h] = on
		redraw(h)
	}
}

func checked(h uintptr) bool { return checkState[h] }

func isCheck(h uintptr) bool { _, ok := checkState[h]; return ok }

func redraw(h uintptr) { procInvalidateRect.Call(h, 0, 1) }

func setStatus(h uintptr, text string, col int) {
	staticColor[h] = col
	setText(h, text)
	redraw(h)
}

func confirm(text string) bool {
	r, _, _ := procMessageBoxW.Call(settingsHwnd, u16ptr(text), u16ptr("PadDeck"), 0x4|0x20)
	return r == 6
}

func runOnUI(f func()) {
	select {
	case uiCalls <- f:
		procPostMessageW.Call(settingsHwnd, wmAppCall, 0, 0)
	default:
	}
}

func enableVisualStyles() {
	manifest := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
<dependency><dependentAssembly><assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/></dependentAssembly></dependency>
</assembly>`
	p := filepath.Join(appDir, "paddeck.manifest")
	if err := os.WriteFile(p, []byte(manifest), 0644); err != nil {
		return
	}
	ac := actCtx{LpSource: u16(p)}
	ac.CbSize = uint32(unsafe.Sizeof(ac))
	h, _, _ := procCreateActCtxW.Call(uintptr(unsafe.Pointer(&ac)))
	if h != 0 && h != ^uintptr(0) {
		var cookie uintptr
		procActivateActCtx.Call(h, uintptr(unsafe.Pointer(&cookie)))
	}
	procInitCommonControls.Call()
}

func mkFont(px float64, weight int, face string) uintptr {
	f, _, _ := procCreateFontW.Call(uintptr(-S(px)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, u16ptr(face))
	return f
}

func ctl(class, text string, style uint32, exStyle uint32, x, y, w, h float64, id int) uintptr {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	hw, _, _ := procCreateWindowExW.Call(uintptr(exStyle), u16ptr(class), u16ptr(text), uintptr(wsChild|wsVisible|style),
		uintptr(S(x)), uintptr(S(y)), uintptr(S(w)), uintptr(S(h)), settingsHwnd, uintptr(id), hInst, 0)
	sendMsg(hw, wmSetFont, fontNormal, 1)
	return hw
}

func label(text string, x, y, w, h float64) uintptr {
	return ctl("STATIC", text, ssNoPrefix, 0, x, y, w, h, 0)
}

func hint(text string, x, y, w, h float64) uintptr {
	l := label(text, x, y, w, h)
	staticColor[l] = colMuted
	return l
}

func button(text string, x, y, w, h float64, id int) uintptr {
	b := ctl("BUTTON", text, bsOwnerDraw|wsTabStop, 0, x, y, w, h, id)
	pushBtns[b] = true
	return b
}

func edit(x, y, w, h float64, id int, style uint32) uintptr {
	return ctl("EDIT", "", wsTabStop|esAutoHScroll|style, wsExClientEdge, x, y, w, h, id)
}

func combo(x, y, w float64, id int, items []string) uintptr {
	c := ctl("COMBOBOX", "", wsTabStop|wsVScroll|cbsDropDownList, 0, x, y, w, 320, id)
	for _, it := range items {
		sendMsg(c, cbAddString, 0, u16ptr(it))
	}
	return c
}

func checkbox(text string, x, y, w float64, id int) uintptr {
	c := ctl("BUTTON", text, bsOwnerDraw|wsTabStop, 0, x, y, w, 22, id)
	checkState[c] = false
	return c
}

type card struct {
	title      string
	x, y, w, h float64
}

func group(text string, x, y, w, h float64) {
	cards = append(cards, card{text, x, y, w, h})
}

func paintBackground(hdc uintptr) {
	var cr rect
	procGetClientRect.Call(settingsHwnd, uintptr(unsafe.Pointer(&cr)))
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&cr)), bgBrush)
	for _, c := range cards {
		r := rect{S(c.x), S(c.y), S(c.x + c.w), S(c.y + c.h)}
		gdiRound(hdc, r, S(14), th.Panel, th.Border, 1)
		drawText(hdc, c.title, rect{r.Left + S(16), r.Top + S(6), r.Right - S(16), r.Top + S(26)}, fontBold, th.Text, dtLeft|dtSingleLine|dtVCenter)
	}
}

// ---------- window creation ----------

const clientW, clientH = 1000, 652

func createSettingsWindow() {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	th = themeByID(getConfigCopy().Theme)
	makeBrushes()
	cursor, _, _ := procLoadCursorW.Call(0, 32512)
	wc := wndClassExW{LpfnWndProc: settingsCB, HInstance: hInst, LpszClassName: u16("PadDeckSettings"), HCursor: cursor, HIcon: trayIcon, HIconSm: trayIcon}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	fontNormal = mkFont(12.5, 400, "Segoe UI")
	fontSmall = mkFont(11, 400, "Segoe UI")
	fontBold = mkFont(13, 600, "Segoe UI")
	fontKey = mkFont(12, 600, "Segoe UI")
	fontMono = mkFont(11.5, 400, "Consolas")
	fontSym = mkFont(15, 700, "Segoe UI")

	style := uint32(wsCaption | wsSysMenu | wsMinimizeBox | 0x02000000) // WS_CLIPCHILDREN
	r := rect{0, 0, S(clientW), S(clientH)}
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), uintptr(style), 0, wsExControlParent)
	w, h := r.Right-r.Left, r.Bottom-r.Top
	sw, _, _ := procGetSystemMetrics.Call(0)
	sh, _, _ := procGetSystemMetrics.Call(1)
	x, y := (int32(sw)-w)/2, (int32(sh)-h)/2
	if y < 0 {
		y = 0
	}
	settingsHwnd, _, _ = procCreateWindowExW.Call(wsExControlParent, u16ptr("PadDeckSettings"), u16ptr("PadDeck – macro pad controller"),
		uintptr(style), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, hInst, 0)
	sendMsg(settingsHwnd, wmSetIcon, 0, trayIcon)
	sendMsg(settingsHwnd, wmSetIcon, 1, trayIcon)

	uiCfg = getConfigCopy()
	if uiCfg.Knobs == 0 {
		uiSel = "k1"
	}

	// --- keypad ---
	group("Your keypad", 10, 6, 500, 400)
	for k := 0; k < maxKnobs; k++ {
		dialBtns[k] = ctl("BUTTON", "", bsOwnerDraw, 0, 0, 0, 56, 56, idDialBase+k)
		knobLbls[k] = ctl("STATIC", "", ssCenter|ssNoPrefix|ssEndEllipsis, 0, 0, 0, 150, 18, 0)
		staticColor[knobLbls[k]] = colMuted
		for p := 0; p < 3; p++ {
			partBtns[k][p] = ctl("BUTTON", "", bsOwnerDraw|wsTabStop, 0, 0, 0, 44, 24, idPartBase+k*3+p)
		}
	}
	for i := 0; i < maxKeys; i++ {
		keyBtns[i] = ctl("BUTTON", "", bsOwnerDraw|wsTabStop, 0, 0, 0, 72, 52, idKeyBase+i)
	}

	// --- editor ---
	group("Edit", 520, 6, 470, 400)
	edHeader = label("", 536, 28, 438, 24)
	sendMsg(edHeader, wmSetFont, fontBold, 1)
	lblMode = label("Knob mode (sets turn left, press and turn right together)", 0, 0, 438, 18)
	var presetNames []string
	for _, p := range knobPresets {
		presetNames = append(presetNames, p.name)
	}
	presetNames = append(presetNames, "Custom (set each part below)")
	cbMode = combo(0, 0, 438, idMode, presetNames)
	lblLabel = label("Label", 0, 0, 438, 18)
	edLabel = edit(0, 0, 438, 24, idLabel, 0)
	lblAction = label("Action", 0, 0, 438, 18)
	var actNames []string
	for _, a := range actionDefs {
		actNames = append(actNames, a.name)
	}
	cbAction = combo(0, 0, 438, idAction, actNames)
	lblP1 = label("", 0, 0, 438, 18)
	edP1 = edit(0, 0, 340, 24, idP1, 0)
	btnRecord = button("Record", 0, 0, 90, 24, idRecord)
	btnBrowse = button("Browse…", 0, 0, 90, 24, idBrowse)
	edText = ctl("EDIT", "", wsTabStop|esMultiline|esWantReturn|esAutoVScroll|wsVScroll, wsExClientEdge, 0, 0, 438, 64, idText)
	lblP2 = label("Arguments (optional)", 0, 0, 438, 18)
	edP2 = edit(0, 0, 438, 24, idP2, 0)
	hintP = hint("", 0, 0, 438, 34)
	sendMsg(hintP, wmSetFont, fontSmall, 1)
	lblTrig = label("Trigger code from keypad", 0, 0, 438, 18)
	cbTrig = combo(0, 0, 160, idTrigger, append([]string{"(none)"}, triggerChoices()...))
	btnLearn = button("Learn", 0, 0, 96, 26, idLearn)
	btnTest = button("Test action", 0, 0, 100, 26, idTest)
	learnMsg = hint("Click Learn, then press this key (or turn/press this knob) on the keypad.", 0, 0, 438, 36)

	// --- program keypad ---
	group("Program the keypad", 10, 412, 500, 232)
	e := hint("Each key and knob gets a unique trigger code (F13–F24, keys normal keyboards don't have). "+
		"The keypad sends the code; PadDeck runs the action. Actions are stored on this PC, so you can change them without re-programming.", 26, 434, 468, 50)
	sendMsg(e, wmSetFont, fontSmall, 1)
	button("Detect keypad", 26, 488, 130, 28, idDetect)
	btnFlash = button("Write trigger codes to keypad", 164, 488, 230, 28, idFlash)
	detStatus = hint("", 26, 522, 468, 18)
	flashLog = ctl("EDIT", "If your keypad isn't detected: set each key to the code shown on it using the keypad's own software, or click Learn on each key.",
		esMultiline|esReadOnly|esAutoVScroll|wsVScroll, wsExClientEdge, 26, 544, 468, 88, 0)
	sendMsg(flashLog, wmSetFont, fontMono, 1)

	// --- settings ---
	group("Settings", 520, 412, 470, 232)
	label("Key rows", 536, 438, 62, 20)
	edRows = edit(600, 434, 40, 24, idRows, esNumber)
	label("Columns", 656, 438, 62, 20)
	edCols = edit(720, 434, 40, 24, idCols, esNumber)
	label("Knobs", 776, 438, 50, 20)
	edKnobs = edit(828, 434, 40, 24, idKnobs, esNumber)
	btnLayout = button("Detect layout", 878, 432, 96, 28, idLayout)
	label("Volume step %", 536, 470, 96, 20)
	edVStep = edit(634, 466, 40, 24, idVStep, esNumber)
	label("Brightness step %", 686, 470, 112, 20)
	edBStep = edit(800, 466, 40, 24, idBStep, esNumber)
	label("App step %", 852, 470, 72, 20)
	edAStep = edit(926, 466, 40, 24, idAStep, esNumber)
	label("Protocol", 536, 502, 60, 20)
	cbProto = combo(600, 498, 150, idProto, []string{"Automatic", "CH57x 8840/8842", "CH57x 8890", "514C:8850 (FD)"})
	label("Theme", 766, 502, 50, 20)
	var themeNames []string
	for _, t := range themes {
		themeNames = append(themeNames, t.Name)
	}
	cbTheme = combo(818, 498, 156, idTheme, themeNames)
	label("Rotation", 536, 534, 60, 20)
	cbRotation = combo(600, 530, 150, idRotation, []string{"0° (as labelled)", "90°", "180°", "270°"})
	chkPause = checkbox("Pause keypad", 766, 531, 200, idPause)
	chkOSD = checkbox("Show on-screen display", 536, 558, 210, idOSD)
	chkAuto = checkbox("Start with Windows", 766, 558, 200, idAuto)
	lastRx = hint("", 26, 384, 468, 18)
	button("Check displays", 536, 586, 128, 28, idDisplays)
	button("Reset trigger codes", 672, 586, 150, 28, idReset)
	button("Exit PadDeck", 830, 586, 144, 28, idExit)
	dispRes = hint("", 536, 614, 438, 30)
	sendMsg(dispRes, wmSetFont, fontSmall, 1)

	primaryBtn = btnFlash
	loadSettingsFields()
	layoutPad()
	updateEditor()
	applyTheme(uiCfg.Theme)
}

func loadSettingsFields() {
	uiLoading = true
	setText(edRows, strconv.Itoa(uiCfg.Rows))
	setText(edCols, strconv.Itoa(uiCfg.Cols))
	setText(edKnobs, strconv.Itoa(uiCfg.Knobs))
	setText(edVStep, strconv.Itoa(uiCfg.VolumeStep))
	setText(edBStep, strconv.Itoa(uiCfg.BrightnessStep))
	setText(edAStep, strconv.Itoa(uiCfg.AppVolumeStep))
	pi := map[string]uintptr{"auto": 0, "884x": 1, "8890": 2, "8850": 3}[uiCfg.Protocol]
	sendMsg(cbProto, cbSetCurSel, pi, 0)
	setCheck(chkOSD, uiCfg.ShowOSD)
	sendMsg(cbTheme, cbSetCurSel, uintptr(themeIndex(uiCfg.Theme)), 0)
	sendMsg(cbRotation, cbSetCurSel, uintptr(uiCfg.Rotation/90), 0)
	setCheck(chkAuto, autostartEnabled())
	setCheck(chkPause, paused.Load())
	uiLoading = false
}

func layoutPad() {
	top := float64(30)
	for k := 0; k < maxKnobs; k++ {
		vis := k < uiCfg.Knobs
		cx := 10 + (500-float64(uiCfg.Knobs)*155)/2 + float64(k)*155
		move(dialBtns[k], S(cx+47), S(top), S(56), S(56))
		move(knobLbls[k], S(cx), S(top+60), S(150), S(18))
		setText(knobLbls[k], knobTitle(k))
		show(dialBtns[k], vis)
		show(knobLbls[k], vis)
		for p := 0; p < 3; p++ {
			move(partBtns[k][p], S(cx+3+float64(p)*49), S(top+82), S(44), S(24))
			show(partBtns[k][p], vis)
			redraw(partBtns[k][p])
		}
		redraw(dialBtns[k])
	}
	ky := top + 122
	if uiCfg.Knobs == 0 {
		ky = top
	}
	n := uiCfg.Rows * uiCfg.Cols
	gridW := float64(uiCfg.Cols)*78 - 6
	kx := 10 + (500-gridW)/2
	for i := 0; i < maxKeys; i++ {
		r, c := i/uiCfg.Cols, i%uiCfg.Cols
		move(keyBtns[i], S(kx+float64(c)*78), S(ky+float64(r)*58), S(72), S(52))
		show(keyBtns[i], i < n)
		redraw(keyBtns[i])
	}
}

func knobPresetIndex(k int) int {
	for i, p := range knobPresets {
		ok := true
		for part := 0; part < 3; part++ {
			if uiCfg.Slots[knobSlotID(k, part)].Action.Type != p.acts[part].id {
				ok = false
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func knobTitle(k int) string {
	if i := knobPresetIndex(k); i >= 0 {
		return knobPresets[i].name
	}
	return "Custom"
}

func slotKnob(id string) (int, int, bool) {
	var k int
	var part string
	if _, err := fmt.Sscanf(id, "n%d_%s", &k, &part); err != nil {
		return 0, 0, false
	}
	for p, n := range knobParts {
		if n == part {
			return k - 1, p, true
		}
	}
	return 0, 0, false
}

func slotButton(id string) uintptr {
	if k, p, ok := slotKnob(id); ok {
		return partBtns[k][p]
	}
	var i int
	if _, err := fmt.Sscanf(id, "k%d", &i); err == nil && i >= 1 && i <= maxKeys {
		return keyBtns[i-1]
	}
	return 0
}

func redrawSlot(id string) {
	if h := slotButton(id); h != 0 {
		redraw(h)
	}
	if k, _, ok := slotKnob(id); ok {
		redraw(dialBtns[k])
		setText(knobLbls[k], knobTitle(k))
	}
}

func selectSlot(id string) {
	old := uiSel
	uiSel = id
	redrawSlot(old)
	redrawSlot(id)
	if learnCancel != nil {
		close(learnCancel)
		learnCancel = nil
	}
	updateEditor()
}

// updateEditor fills and lays out the right-hand editor for the selected slot.
func updateEditor() {
	s := uiCfg.Slots[uiSel]
	if s == nil {
		return
	}
	uiLoading = true
	defer func() { uiLoading = false }()
	k, p, isKnob := slotKnob(uiSel)
	if isKnob {
		setText(edHeader, fmt.Sprintf("Knob %d  ·  %s", k+1, partNames[p]))
		idx := knobPresetIndex(k)
		if idx < 0 {
			idx = len(knobPresets)
		}
		sendMsg(cbMode, cbSetCurSel, uintptr(idx), 0)
	} else {
		setText(edHeader, "Key "+strings.TrimPrefix(uiSel, "k"))
	}
	setText(edLabel, s.Label)
	for i, a := range actionDefs {
		if a.id == s.Action.Type {
			sendMsg(cbAction, cbSetCurSel, uintptr(i), 0)
		}
	}
	sendMsg(cbTrig, cbSetCurSel, uintptr(triggerIndex(s.Trigger)), 0)

	x := float64(536)
	y := float64(58)
	cx, cw := x+104, float64(334) // control column
	row := func(lbl uintptr, text string) {
		setText(lbl, text)
		move(lbl, S(x), S(y+4), S(100), S(20))
		show(lbl, true)
	}
	for _, h := range []uintptr{lblMode, cbMode, lblP1, edP1, btnRecord, btnBrowse, edText, lblP2, edP2, hintP} {
		show(h, false)
	}
	if isKnob {
		row(lblMode, "Knob mode")
		move(cbMode, S(cx), S(y), S(cw), S(320))
		show(cbMode, true)
		y += 34
	}
	row(lblLabel, "Label")
	move(edLabel, S(cx), S(y), S(cw), S(24))
	y += 34
	row(lblAction, "Action")
	move(cbAction, S(cx), S(y), S(cw), S(320))
	y += 34

	t := s.Action.Type
	switch t {
	case "shortcut":
		row(lblP1, "Shortcut")
		setText(edP1, s.Action.Keys)
		move(edP1, S(cx), S(y), S(cw-98), S(24))
		move(btnRecord, S(cx+cw-92), S(y-1), S(92), S(26))
		show(edP1, true)
		show(btnRecord, true)
		y += 34
	case "text":
		row(lblP1, "Text to type")
		setText(edText, strings.ReplaceAll(strings.ReplaceAll(s.Action.Text, "\r\n", "\n"), "\n", "\r\n"))
		move(edText, S(cx), S(y), S(cw), S(70))
		show(edText, true)
		y += 78
	case "launch":
		row(lblP1, "Open")
		setText(edP1, s.Action.Path)
		move(edP1, S(cx), S(y), S(cw-98), S(24))
		move(btnBrowse, S(cx+cw-92), S(y-1), S(92), S(26))
		show(edP1, true)
		show(btnBrowse, true)
		y += 34
		row(lblP2, "Arguments")
		setText(edP2, s.Action.Args)
		move(edP2, S(cx), S(y), S(cw), S(24))
		show(edP2, true)
		y += 34
	case "command":
		row(lblP1, "Command")
		setText(edP1, s.Action.Path)
		move(edP1, S(cx), S(y), S(cw), S(24))
		show(edP1, true)
		y += 34
	}
	h := ""
	switch {
	case strings.HasPrefix(t, "appvol"):
		h = "Changes only the volume of the app in front (e.g. a game or Spotify)."
	case strings.HasPrefix(t, "brightness"):
		h = "Laptop screens use Windows' brightness; external monitors use DDC/CI (enable it in the monitor's menu)."
	case t == "launch":
		h = "Program, file, folder or URL. Tip: ms-settings:sound opens Sound settings, shell:Downloads opens Downloads."
	case t == "shortcut":
		h = "Use names like Ctrl, Shift, Alt, Win, A–Z, F1–F24, Enter, Esc, Tab, Space, Up, PrintScreen."
	case t == "command":
		h = "Runs in cmd.exe without a window, e.g.  shutdown /s /t 60"
	case isKnob:
		h = "The knob mode sets turn left, press and turn right together. Use Custom to set each part yourself."
	}
	if h != "" {
		setText(hintP, h)
		move(hintP, S(cx), S(y-2), S(cw), S(34))
		show(hintP, true)
		y += 38
	}
	y += 6
	row(lblTrig, "Trigger code")
	move(cbTrig, S(cx), S(y), S(160), S(320))
	move(btnLearn, S(cx+166), S(y-1), S(80), S(26))
	move(btnTest, S(cx+252), S(y-1), S(82), S(26))
	y += 34
	move(learnMsg, S(cx), S(y), S(cw), S(40))
	if learnCancel == nil {
		setText(btnLearn, "Learn")
		setStatus(learnMsg, "Click Learn, then press this key (or turn/press this knob) on the keypad.", colMuted)
	}
	procRedrawWindow.Call(settingsHwnd, 0, 0, 0x1|0x4|0x80) // INVALIDATE|ERASE|ALLCHILDREN
}

// ---------- saving ----------

func scheduleSave() {
	if uiLoading {
		return
	}
	procSetTimer.Call(settingsHwnd, timerSave, 450, 0)
}

func saveNow() {
	procKillTimer.Call(settingsHwnd, timerSave)
	for id, s := range uiCfg.Slots {
		if s.Trigger == "" {
			continue
		}
		code, err := parseTrigger(s.Trigger)
		if err != nil {
			if id == uiSel {
				setStatus(learnMsg, "Invalid trigger code: "+err.Error(), colBad)
			}
			continue
		}
		if c := codeString(code); c != s.Trigger {
			s.Trigger = c
			if id == uiSel {
				uiLoading = true
				sendMsg(cbTrig, cbSetCurSel, uintptr(triggerIndex(c)), 0)
				uiLoading = false
			}
		}
	}
	b, _ := json.Marshal(uiCfg)
	var c Config
	_ = json.Unmarshal(b, &c)
	if err := setConfig(&c); err != nil {
		logf("save: %v", err)
	}
}

var procGetFocus = modUser32.NewProc("GetFocus")

func getFocus() uintptr { r, _, _ := procGetFocus.Call(); return r }

// ---------- show / hide ----------

func showSettings() {
	if settingsHwnd == 0 {
		return
	}
	setCheck(chkPause, paused.Load())
	setCheck(chkAuto, autostartEnabled())
	if r, _, _ := procIsIconic.Call(settingsHwnd); r != 0 {
		procShowWindow.Call(settingsHwnd, 9)
	} else {
		procShowWindow.Call(settingsHwnd, 5)
	}
	procSetForegroundWindow.Call(settingsHwnd)
}

func hideSettings() {
	applyNumbers()
	procShowWindow.Call(settingsHwnd, swHide)
}

// ---------- drawing ----------

func gdiRound(hdc uintptr, r rect, rad int32, fill, border uintptr, bw int32) {
	br, _, _ := procCreateSolidBrush.Call(fill)
	pen, _, _ := procCreatePen.Call(0, uintptr(bw), border)
	ob, _, _ := procSelectObject.Call(hdc, br)
	op, _, _ := procSelectObject.Call(hdc, pen)
	procRoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(rad), uintptr(rad))
	procSelectObject.Call(hdc, ob)
	procSelectObject.Call(hdc, op)
	procDeleteObject.Call(br)
	procDeleteObject.Call(pen)
}

func drawText(hdc uintptr, s string, r rect, font, col uintptr, flags uintptr) {
	of, _, _ := procSelectObject.Call(hdc, font)
	procSetBkMode.Call(hdc, 1)
	procSetTextColor.Call(hdc, col)
	procDrawTextW.Call(hdc, u16ptr(s), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags)
	procSelectObject.Call(hdc, of)
}

func drawItem(d *drawItemStruct) {
	hdc := d.HDC
	r := d.RcItem
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), panelBrush)
	id := int(d.CtlID)
	focus := d.ItemState&0x10 != 0
	pressed := d.ItemState&0x1 != 0
	disabled := d.ItemState&0x4 != 0
	in := rect{r.Left + 1, r.Top + 1, r.Right - 1, r.Bottom - 1}
	switch {
	case pushBtns[d.HwndItem]:
		fill, border, col := th.Key, th.KeyBorder, th.Text
		if d.HwndItem == primaryBtn {
			fill, border, col = th.Accent, th.Accent, th.AccentText
		}
		if pressed {
			fill = th.Flash
			col = th.Text
		}
		if focus {
			border = th.Accent
		}
		if disabled {
			col = th.Muted
		}
		gdiRound(hdc, in, S(8), fill, border, 1)
		drawText(hdc, getText(d.HwndItem), in, fontNormal, col, dtCenter|dtVCenter|dtSingleLine|dtEndEllipsis)
	case isCheck(d.HwndItem):
		bs := S(16)
		top := (r.Top + r.Bottom - bs) / 2
		box := rect{r.Left + 1, top, r.Left + 1 + bs, top + bs}
		if checked(d.HwndItem) {
			gdiRound(hdc, box, S(5), th.Accent, th.Accent, 1)
			pen, _, _ := procCreatePen.Call(0, uintptr(S(2)), th.AccentText)
			op, _, _ := procSelectObject.Call(hdc, pen)
			procMoveToEx.Call(hdc, uintptr(box.Left+S(4)), uintptr(box.Top+S(8)), 0)
			procLineTo.Call(hdc, uintptr(box.Left+S(7)), uintptr(box.Top+S(11)))
			procLineTo.Call(hdc, uintptr(box.Left+S(12)), uintptr(box.Top+S(5)))
			procSelectObject.Call(hdc, op)
			procDeleteObject.Call(pen)
		} else {
			border := th.KeyBorder
			if focus {
				border = th.Accent
			}
			gdiRound(hdc, box, S(5), th.Input, border, 1)
		}
		drawText(hdc, getText(d.HwndItem), rect{box.Right + S(8), r.Top, r.Right, r.Bottom}, fontNormal, th.Text, dtLeft|dtVCenter|dtSingleLine)
	case id >= idKeyBase && id < idKeyBase+maxKeys:
		slot := keySlotID(id - idKeyBase)
		s := uiCfg.Slots[slot]
		fill, border, bw := th.Key, th.KeyBorder, int32(1)
		if slot == flashSlot {
			fill = th.Flash
		}
		if slot == uiSel {
			border, bw = th.Accent, S(2)
		} else if focus {
			border = th.Muted
		}
		gdiRound(hdc, in, S(10), fill, border, bw)
		lbl := s.Label
		if lbl == "" {
			lbl = actionShortName(s.Action.Type)
		}
		pad := S(7)
		drawText(hdc, lbl, rect{in.Left + pad, in.Top + S(5), in.Right - pad, in.Bottom - S(18)}, fontKey, th.Text, 0x10|dtEndEllipsis) // WORDBREAK
		trig := s.Trigger
		if trig == "" {
			trig = "no trigger"
		}
		drawText(hdc, trig, rect{in.Left + pad, in.Bottom - S(18), in.Right - pad, in.Bottom - S(3)}, fontSmall, th.Muted, dtLeft|dtSingleLine|dtEndEllipsis)
	case id >= idDialBase && id < idDialBase+maxKnobs:
		k := id - idDialBase
		sel := strings.HasPrefix(uiSel, fmt.Sprintf("n%d_", k+1))
		fl := strings.HasPrefix(flashSlot, fmt.Sprintf("n%d_", k+1))
		ring, rw := th.KeyBorder, S(2)
		if sel {
			ring, rw = th.Accent, S(3)
		}
		fill := th.Dial
		if fl {
			fill = th.Flash
		}
		br, _, _ := procCreateSolidBrush.Call(fill)
		pen, _, _ := procCreatePen.Call(0, uintptr(rw), ring)
		ob, _, _ := procSelectObject.Call(hdc, br)
		op, _, _ := procSelectObject.Call(hdc, pen)
		m := S(3)
		procEllipse.Call(hdc, uintptr(r.Left+m), uintptr(r.Top+m), uintptr(r.Right-m), uintptr(r.Bottom-m))
		procSelectObject.Call(hdc, ob)
		procSelectObject.Call(hdc, op)
		procDeleteObject.Call(br)
		procDeleteObject.Call(pen)
		ind, _, _ := procCreatePen.Call(0, uintptr(S(4)), th.Accent)
		op, _, _ = procSelectObject.Call(hdc, ind)
		cx := (r.Left + r.Right) / 2
		procMoveToEx.Call(hdc, uintptr(cx), uintptr(r.Top+S(12)), 0)
		procLineTo.Call(hdc, uintptr(cx), uintptr(r.Top+S(24)))
		procSelectObject.Call(hdc, op)
		procDeleteObject.Call(ind)
	case id >= idPartBase && id < idPartBase+maxKnobs*3:
		k, p := (id-idPartBase)/3, (id-idPartBase)%3
		slot := knobSlotID(k, p)
		fill, border, bw := th.Key, th.KeyBorder, int32(1)
		col := th.Muted
		if slot == flashSlot {
			fill = th.Flash
		}
		if slot == uiSel {
			border, bw, col = th.Accent, S(2), th.Accent
		} else if focus {
			border = th.Muted
		}
		gdiRound(hdc, in, S(8), fill, border, bw)
		sym := []string{"←", "•", "→"}[p]
		drawText(hdc, sym, in, fontSym, col, dtCenter|dtVCenter|dtSingleLine)
	}
}

// ---------- events ----------

func settingsProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case wmClose:
		hideSettings()
		return 0
	case wmAppShow:
		showSettings()
		return 0
	case wmAppCall:
		for {
			select {
			case f := <-uiCalls:
				func() { defer recoverLog("ui call"); f() }()
				continue
			default:
			}
			break
		}
		return 0
	case wmAppFired:
		firedMu.Lock()
		slot, trig := firedSlot, firedCode
		firedMu.Unlock()
		old := flashSlot
		flashSlot = slot
		redrawSlot(old)
		redrawSlot(slot)
		setText(lastRx, "Last received: "+trig+" → "+slotName(slot))
		procSetTimer.Call(hwnd, timerFlash, 350, 0)
		return 0
	case wmTimer:
		switch wParam {
		case timerSave:
			saveNow()
			return 0
		case timerFlash:
			procKillTimer.Call(hwnd, timerFlash)
			old := flashSlot
			flashSlot = ""
			redrawSlot(old)
			return 0
		}
	case wmDrawItem:
		func() {
			defer recoverLog("draw")
			drawItem((*drawItemStruct)(unsafe.Pointer(lParam)))
		}()
		return 1
	case 0x0014: // WM_ERASEBKGND
		paintBackground(wParam)
		return 1
	case wmCtlColorStatic:
		if lParam == flashLog {
			procSetBkColor.Call(wParam, th.Input)
			procSetTextColor.Call(wParam, th.InputText)
			return inputBrush
		}
		procSetBkColor.Call(wParam, th.Panel)
		procSetTextColor.Call(wParam, themeColor(staticColor[lParam]))
		return panelBrush
	case 0x0133, 0x0134: // WM_CTLCOLOREDIT, WM_CTLCOLORLISTBOX
		procSetBkColor.Call(wParam, th.Input)
		procSetTextColor.Call(wParam, th.InputText)
		return inputBrush
	case wmCommand:
		func() {
			defer recoverLog("command")
			onCommand(int(wParam&0xFFFF), uint32(wParam>>16)&0xFFFF, lParam)
		}()
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func slotName(id string) string {
	if k, p, ok := slotKnob(id); ok {
		return fmt.Sprintf("knob %d %s", k+1, strings.ToLower(partNames[p]))
	}
	return "key " + strings.TrimPrefix(id, "k")
}

func onCommand(id int, code uint32, h uintptr) {
	s := uiCfg.Slots[uiSel]
	if isCheck(h) {
		if code != bnClicked && code != 5 { // BN_DOUBLECLICKED
			return
		}
		setCheck(h, !checked(h))
	}
	switch {
	case id == 2: // Esc
		hideSettings()
		return
	case id >= idKeyBase && id < idKeyBase+maxKeys:
		selectSlot(keySlotID(id - idKeyBase))
		return
	case id >= idDialBase && id < idDialBase+maxKnobs:
		selectSlot(knobSlotID(id-idDialBase, 1))
		return
	case id >= idPartBase && id < idPartBase+maxKnobs*3:
		selectSlot(knobSlotID((id-idPartBase)/3, (id-idPartBase)%3))
		return
	}
	if uiLoading {
		return
	}
	switch id {
	case idMode:
		if code != cbnSelChange {
			return
		}
		k, _, ok := slotKnob(uiSel)
		i := int(sendMsg(cbMode, cbGetCurSel, 0, 0))
		if !ok || i < 0 || i >= len(knobPresets) {
			return
		}
		for p := 0; p < 3; p++ {
			sl := uiCfg.Slots[knobSlotID(k, p)]
			sl.Action = Action{Type: knobPresets[i].acts[p].id}
			sl.Label = knobPresets[i].acts[p].name
			redraw(partBtns[k][p])
		}
		setText(knobLbls[k], knobTitle(k))
		updateEditor()
		scheduleSave()
	case idLabel:
		if code == enChange {
			s.Label = getText(edLabel)
			redrawSlot(uiSel)
			scheduleSave()
		}
	case idAction:
		if code != cbnSelChange {
			return
		}
		i := int(sendMsg(cbAction, cbGetCurSel, 0, 0))
		if i < 0 || i >= len(actionDefs) {
			return
		}
		prev := actionShortName(s.Action.Type)
		s.Action.Type = actionDefs[i].id
		if s.Label == "" || s.Label == prev {
			s.Label = actionShortName(s.Action.Type)
		}
		updateEditor()
		redrawSlot(uiSel)
		scheduleSave()
	case idP1:
		if code == enChange {
			switch s.Action.Type {
			case "shortcut":
				s.Action.Keys = getText(edP1)
			case "launch", "command":
				s.Action.Path = getText(edP1)
			}
			scheduleSave()
		}
	case idText:
		if code == enChange {
			s.Action.Text = strings.ReplaceAll(getText(edText), "\r\n", "\n")
			scheduleSave()
		}
	case idP2:
		if code == enChange {
			s.Action.Args = getText(edP2)
			scheduleSave()
		}
	case idTrigger:
		if code == cbnSelChange {
			i := int(sendMsg(cbTrig, cbGetCurSel, 0, 0))
			trig := ""
			if ch := triggerChoices(); i >= 1 && i <= len(ch) {
				trig = ch[i-1]
			}
			assignTrigger(uiSel, trig)
			saveNow()
		}
	case idLearn:
		startLearn("trigger")
	case idRecord:
		startLearn("shortcut")
	case idBrowse:
		if p := browseFile(); p != "" {
			setText(edP1, p) // triggers EN_CHANGE → saves
		}
	case idTest:
		saveNow()
		a := s.Action
		setStatus(learnMsg, "Running the action…", colMuted)
		go func() { time.Sleep(500 * time.Millisecond); runActionAsync(a) }()
	case idDetect:
		setStatus(detStatus, "Looking for the keypad…", colMuted)
		go func() {
			t := pickTarget(enumCH57x())
			runOnUI(func() {
				if t != nil {
					setStatus(detStatus, fmt.Sprintf("Found keypad (USB %04X:%04X), protocol %s", t.VID, t.PID, protocolFor(t.VID, t.PID, uiCfg.Protocol)), colGood)
				} else {
					setStatus(detStatus, "No supported keypad found (see the note below).", colWarn)
				}
			})
		}()
	case idFlash:
		if !confirm("Write PadDeck's trigger codes to the keypad now?\n\nThis replaces what is stored on the keypad (layer 1).") {
			return
		}
		saveNow()
		setText(flashLog, "Programming… this takes about 30 seconds. Don't press keys on the pad meanwhile.")
		procEnableWindow.Call(btnFlash, 0)
		c := getConfigCopy()
		go func() {
			done := make(chan struct{})
			var lines []string
			var err error
			go func() {
				defer recoverLog("flash")
				lines, err = flashPad(c)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(90 * time.Second):
				err = fmt.Errorf("keypad did not respond (timeout)")
			}
			runOnUI(func() {
				procEnableWindow.Call(btnFlash, 1)
				out := strings.Join(lines, "\r\n")
				if err != nil {
					out += "\r\n\r\nError: " + err.Error()
				} else {
					out += "\r\n\r\nSuccess. Press keys on the pad – they light up in the keypad picture."
				}
				setText(flashLog, strings.TrimLeft(out, "\r\n"))
			})
		}()
	case idRows, idCols, idKnobs, idVStep, idBStep, idAStep:
		if code != enKillFocus {
			return
		}
		applyNumbers()
	case idProto:
		if code == cbnSelChange {
			uiCfg.Protocol = []string{"auto", "884x", "8890", "8850"}[sendMsg(cbProto, cbGetCurSel, 0, 0)%4]
			scheduleSave()
		}
	case idRotation:
		if code == cbnSelChange {
			uiCfg.Rotation = int(sendMsg(cbRotation, cbGetCurSel, 0, 0)%4) * 90
			setStatus(dispRes, "Rotation changed. Click \"Write trigger codes to keypad\" to apply it.", colWarn)
			scheduleSave()
		}
	case idLayout:
		startLayoutDetect()
	case idTheme:
		if code == cbnSelChange {
			i := int(sendMsg(cbTheme, cbGetCurSel, 0, 0))
			if i >= 0 && i < len(themes) {
				uiCfg.Theme = themes[i].ID
				applyTheme(uiCfg.Theme)
				scheduleSave()
			}
		}
	case idOSD:
		uiCfg.ShowOSD = checked(chkOSD)
		scheduleSave()
	case idAuto:
		if err := setAutostart(checked(chkAuto)); err != nil {
			setStatus(dispRes, "Could not change autostart: "+err.Error(), colBad)
		}
		setCheck(chkAuto, autostartEnabled())
	case idPause:
		paused.Store(checked(chkPause))
	case idDisplays:
		setStatus(dispRes, "Checking displays…", colMuted)
		go func() {
			brMu.Lock()
			ensureDisplays()
			brMu.Unlock()
			info := brightnessInfo()
			runOnUI(func() {
				var parts []string
				if n, _ := info["builtin"].(int); n > 0 {
					parts = append(parts, "built-in screen")
				}
				if n, _ := info["ddc"].(int); n > 0 {
					names, _ := info["ddcNames"].([]string)
					parts = append(parts, fmt.Sprintf("%d external monitor(s) (%s)", n, strings.Join(names, ", ")))
				}
				if len(parts) > 0 {
					setStatus(dispRes, "Brightness control: "+strings.Join(parts, " + "), colGood)
				} else {
					setStatus(dispRes, "No display with software brightness. Enable DDC/CI in your monitor's menu.", colWarn)
				}
			})
		}()
	case idReset:
		if !confirm("Reset every trigger code to PadDeck's defaults (F13–F24)?\nActions stay unchanged. Re-program the keypad afterwards.") {
			return
		}
		for id, sl := range uiCfg.Slots {
			sl.Trigger = defaultTrigger(id)
		}
		saveNow()
		layoutPad()
		updateEditor()
	case idExit:
		if confirm("Exit PadDeck? The keypad actions stop until you start it again.") {
			saveNow()
			quitApp()
		}
	}
}

func applyNumbers() {
	num := func(h uintptr, lo, hi, def int) int {
		v, err := strconv.Atoi(strings.TrimSpace(getText(h)))
		if err != nil {
			v = def
		}
		if v < lo {
			v = lo
		}
		if v > hi {
			v = hi
		}
		return v
	}
	uiCfg.Rows = num(edRows, 1, 4, uiCfg.Rows)
	uiCfg.Cols = num(edCols, 1, 6, uiCfg.Cols)
	uiCfg.Knobs = num(edKnobs, 0, maxKnobs, uiCfg.Knobs)
	for uiCfg.Rows*uiCfg.Cols > maxKeys {
		uiCfg.Rows--
	}
	uiCfg.VolumeStep = num(edVStep, 1, 25, uiCfg.VolumeStep)
	uiCfg.BrightnessStep = num(edBStep, 1, 50, uiCfg.BrightnessStep)
	uiCfg.AppVolumeStep = num(edAStep, 1, 50, uiCfg.AppVolumeStep)
	// keep the selection valid
	if k, _, ok := slotKnob(uiSel); ok && k >= uiCfg.Knobs {
		uiSel = "k1"
	}
	var i int
	if _, err := fmt.Sscanf(uiSel, "k%d", &i); err == nil && i > uiCfg.Rows*uiCfg.Cols {
		if uiCfg.Knobs > 0 {
			uiSel = "n1_ccw"
		} else {
			uiSel = "k1"
		}
	}
	loadSettingsFields()
	layoutPad()
	updateEditor()
	saveNow()
}

func startLearn(mode string) {
	if learnCancel != nil { // second click cancels
		close(learnCancel)
		learnCancel = nil
		setText(btnLearn, "Learn")
		setText(btnRecord, "Record")
		setStatus(learnMsg, "Cancelled.", colMuted)
		return
	}
	learnCancel = make(chan struct{})
	learnMode = mode
	cancel := learnCancel
	slot := uiSel
	if mode == "trigger" {
		setText(btnLearn, "Cancel")
		setStatus(learnMsg, "Waiting… press the key or turn/press the knob on your keypad (20 s).", colAccent)
	} else {
		setText(btnRecord, "Cancel")
		setStatus(learnMsg, "Press the shortcut now (on any keyboard)…", colAccent)
	}
	go func() {
		code, ok := learn(cancel, 20*time.Second)
		runOnUI(func() {
			if learnCancel == cancel {
				learnCancel = nil
			}
			setText(btnLearn, "Learn")
			setText(btnRecord, "Record")
			if !ok {
				select {
				case <-cancel:
				default:
					setStatus(learnMsg, "Nothing received. Try again.", colWarn)
				}
				return
			}
			trig := codeString(code)
			sl := uiCfg.Slots[slot]
			if mode == "shortcut" {
				sl.Action.Keys = trig
				if uiSel == slot {
					setText(edP1, trig)
				}
				setStatus(learnMsg, "Recorded "+trig+".", colGood)
				scheduleSave()
				return
			}
			if !allowedTrigger(code) {
				setStatus(learnMsg, "The keypad sent "+trig+", a normal keyboard key. Click \"Write trigger codes to keypad\" first so it sends F13–F24, then Learn again.", colWarn)
				return
			}
			assignTrigger(slot, trig)
			if uiSel == slot {
				uiLoading = true
				sendMsg(cbTrig, cbSetCurSel, uintptr(triggerIndex(trig)), 0)
				uiLoading = false
			}
			setStatus(learnMsg, "Learned "+trig+".", colGood)
			saveNow()
		})
	}()
}

func browseFile() string {
	buf := make([]uint16, 1024)
	filter := append(utf16.Encode([]rune("Programs and shortcuts\x00*.exe;*.lnk;*.bat;*.cmd;*.ps1\x00All files\x00*.*\x00")), 0)
	ofn := openFileName{HwndOwner: settingsHwnd, LpstrFilter: &filter[0], LpstrFile: &buf[0], NMaxFile: uint32(len(buf)),
		Flags: 0x1000 | 0x8 | 0x80000, LpstrTitle: u16("Choose a program or file")}
	ofn.LStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func notifyFired() {
	if settingsHwnd != 0 {
		procPostMessageW.Call(settingsHwnd, wmAppFired, 0, 0)
	}
}

// assignTrigger gives a slot a trigger code; a code can only belong to one slot.
func assignTrigger(slot, trig string) {
	if trig != "" {
		for id, o := range uiCfg.Slots {
			if id != slot && o.Trigger == trig {
				o.Trigger = ""
				redrawSlot(id)
				setStatus(learnMsg, trig+" was moved here from "+slotName(id)+".", colWarn)
			}
		}
	}
	uiCfg.Slots[slot].Trigger = trig
	redrawSlot(slot)
}

// startLayoutDetect asks for the top-left and top-right keys and works out how the pad numbers its keys.
func startLayoutDetect() {
	if learnCancel != nil {
		close(learnCancel)
		learnCancel = nil
		setText(btnLayout, "Detect layout")
		setStatus(dispRes, "Cancelled.", colMuted)
		return
	}
	applyNumbers()
	cancel := make(chan struct{})
	learnCancel = cancel
	setText(btnLayout, "Cancel")
	setStatus(dispRes, "Press the TOP-LEFT key on the keypad (as it sits on your desk)…", colAccent)
	R, C, curRot := uiCfg.Rows, uiCfg.Cols, uiCfg.Rotation
	triggers := map[string]int{}
	for i := 0; i < R*C; i++ {
		triggers[uiCfg.Slots[keySlotID(i)].Trigger] = keyDeviceID(R, C, curRot, i/C, i%C)
	}
	toID := func(code uint32) (int, bool) {
		if id, ok := factoryKeyID(uint16(code&0xFFFF), code>>16); ok {
			return id, true
		}
		id, ok := triggers[codeString(code)]
		return id, ok
	}
	go func() {
		done := func(msg string, col int) {
			runOnUI(func() {
				if learnCancel == cancel {
					learnCancel = nil
				}
				setText(btnLayout, "Detect layout")
				setStatus(dispRes, msg, col)
			})
		}
		c1, ok := learn(cancel, 30*time.Second)
		if !ok {
			done("Layout detection stopped.", colMuted)
			return
		}
		runOnUI(func() { setStatus(dispRes, "Now press the TOP-RIGHT key…", colAccent) })
		c2, ok := learn(cancel, 30*time.Second)
		if !ok {
			done("Layout detection stopped.", colMuted)
			return
		}
		id1, ok1 := toID(c1)
		id2, ok2 := toID(c2)
		if !ok1 || !ok2 {
			done("Couldn't read the key numbers ("+codeString(c1)+", "+codeString(c2)+"). Check the rows and columns.", colBad)
			return
		}
		rot, ok := detectRotation(R, C, id1, id2)
		if !ok {
			done(fmt.Sprintf("Keys %d and %d don't fit a %d×%d grid. Check rows and columns.", id1, id2, R, C), colBad)
			return
		}
		runOnUI(func() {
			uiCfg.Rotation = rot
			uiLoading = true
			sendMsg(cbRotation, cbSetCurSel, uintptr(rot/90), 0)
			uiLoading = false
			saveNow()
		})
		done(fmt.Sprintf("Layout found: rotation %d°. Now click \"Write trigger codes to keypad\".", rot), colGood)
	}()
}
