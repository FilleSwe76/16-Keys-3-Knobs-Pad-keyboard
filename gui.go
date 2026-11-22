//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"syscall"
	"unsafe"
)

var (
	mainHwnd          uintptr
	osdHwnd           uintptr
	trayIcon          uintptr
	taskbarCreatedMsg uint32
	wndProcCB         = syscall.NewCallback(wndProc)
	dpiScale          = 1.0

	osdMu    sync.Mutex
	osdTitle string
	osdPct   int
	osdMuted bool
	osdW     int32
	osdH     int32
)

const (
	menuOpen  = 1
	menuPause = 2
	menuExit  = 3
)

func createWindows() {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	if procGetDpiForSystem.Find() == nil {
		if d, _, _ := procGetDpiForSystem.Call(); d > 0 {
			dpiScale = float64(d) / 96
		}
	}
	cursor, _, _ := procLoadCursorW.Call(0, 32512)
	for _, name := range []string{"PadDeckMain", "PadDeckOSD"} {
		wc := wndClassExW{LpfnWndProc: wndProcCB, HInstance: hInst, LpszClassName: u16(name), HCursor: cursor}
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	}
	mainHwnd, _, _ = procCreateWindowExW.Call(0, u16ptr("PadDeckMain"), u16ptr("PadDeck"), wsOverlapped,
		0, 0, 0, 0, 0, 0, hInst, 0)
	osdW = int32(340 * dpiScale)
	osdH = int32(76 * dpiScale)
	osdHwnd, _, _ = procCreateWindowExW.Call(wsExTopmost|wsExToolWindow|wsExLayered|wsExNoActivate|wsExTransparent,
		u16ptr("PadDeckOSD"), u16ptr(""), wsPopup, 0, 0, uintptr(osdW), uintptr(osdH), 0, 0, hInst, 0)
	procSetLayeredWindowAttributes.Call(osdHwnd, 0, 238, 2)
	rad := uintptr(18 * dpiScale)
	rgn, _, _ := procCreateRoundRectRgn.Call(0, 0, uintptr(osdW+1), uintptr(osdH+1), rad, rad)
	procSetWindowRgn.Call(osdHwnd, rgn, 1)
	r, _, _ := procRegisterWindowMessageW.Call(u16ptr("TaskbarCreated"))
	taskbarCreatedMsg = uint32(r)
	trayIcon = makeIcon(32)
	addTrayIcon()
}

func addTrayIcon() {
	nid := notifyIconData{HWnd: mainHwnd, UID: 1, UFlags: nifMessage | nifIcon | nifTip, UCallbackMessage: wmTray, HIcon: trayIcon}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	copy(nid.SzTip[:], syscall.StringToUTF16("PadDeck – macro pad controller"))
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
}

func removeTrayIcon() {
	nid := notifyIconData{HWnd: mainHwnd, UID: 1}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

func messageLoop() {
	var m msgT
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if settingsHwnd != 0 {
			if r, _, _ := procIsDialogMessageW.Call(settingsHwnd, uintptr(unsafe.Pointer(&m))); r != 0 {
				continue
			}
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func quitApp() { procPostMessageW.Call(mainHwnd, wmAppQuit, 0, 0) }

func showTrayMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	procAppendMenuW.Call(menu, mfString, menuOpen, u16ptr("Open settings"))
	pf := uintptr(mfString)
	if paused.Load() {
		pf |= mfChecked
	}
	procAppendMenuW.Call(menu, pf, menuPause, u16ptr("Pause keypad"))
	procAppendMenuW.Call(menu, mfSeparator, 0, 0)
	procAppendMenuW.Call(menu, mfString, menuExit, u16ptr("Exit PadDeck"))
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(mainHwnd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightButton, uintptr(pt.X), uintptr(pt.Y), 0, mainHwnd, 0)
	procPostMessageW.Call(mainHwnd, wmNull, 0, 0)
	procDestroyMenu.Call(menu)
	switch cmd {
	case menuOpen:
		openSettings()
	case menuPause:
		paused.Store(!paused.Load())
		if paused.Load() {
			osdShow("PadDeck paused", -1, false)
		} else {
			osdShow("PadDeck active", -1, false)
		}
	case menuExit:
		quitApp()
	}
}

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case wmTray:
		switch uint32(lParam) & 0xFFFF {
		case wmLButtonUp:
			openSettings()
		case wmRButtonUp, wmContextMenu:
			showTrayMenu()
		}
		return 0
	case wmAppOSD:
		showOSDNow()
		return 0
	case wmAppQuit:
		procDestroyWindow.Call(mainHwnd)
		return 0
	case wmPaint:
		if hwnd == osdHwnd {
			paintOSD(hwnd)
			return 0
		}
	case wmTimer:
		if hwnd == osdHwnd {
			procKillTimer.Call(hwnd, 1)
			procShowWindow.Call(hwnd, swHide)
			return 0
		}
	case wmDestroy:
		if hwnd == mainHwnd {
			procPostQuitMessage.Call(0)
			return 0
		}
	default:
		if taskbarCreatedMsg != 0 && uint32(msg) == taskbarCreatedMsg {
			addTrayIcon()
			return 0
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// ---------- on-screen display ----------

func osdShow(title string, pct int, muted bool) {
	cfgMu.RLock()
	show := cfg.ShowOSD
	cfgMu.RUnlock()
	if !show || osdHwnd == 0 {
		return
	}
	osdMu.Lock()
	osdTitle, osdPct, osdMuted = title, pct, muted
	osdMu.Unlock()
	procPostMessageW.Call(mainHwnd, wmAppOSD, 0, 0)
}

func showOSDNow() {
	sw, _, _ := procGetSystemMetrics.Call(0)
	sh, _, _ := procGetSystemMetrics.Call(1)
	x := (int32(sw) - osdW) / 2
	y := int32(sh) - osdH - int32(110*dpiScale)
	procSetWindowPos.Call(osdHwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(osdW), uintptr(osdH), swpNoActivate|swpShowWindow)
	procInvalidateRect.Call(osdHwnd, 0, 1)
	procSetTimer.Call(osdHwnd, 1, 1400, 0)
}

func paintOSD(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	osdMu.Lock()
	title, p, muted := osdTitle, osdPct, osdMuted
	osdMu.Unlock()

	s := func(v float64) int32 { return int32(v * dpiScale) }
	fill := func(r rect, c uintptr) {
		b, _, _ := procCreateSolidBrush.Call(c)
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), b)
		procDeleteObject.Call(b)
	}
	fill(rect{0, 0, osdW, osdH}, th.Panel)

	font, _, _ := procCreateFontW.Call(uintptr(-s(19)), 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 5, 0, u16ptr("Segoe UI"))
	old, _, _ := procSelectObject.Call(hdc, font)
	procSetBkMode.Call(hdc, 1)
	procSetTextColor.Call(hdc, th.Text)

	pad := s(20)
	if p < 0 {
		tr := rect{pad, 0, osdW - pad, osdH}
		procDrawTextW.Call(hdc, u16ptr(title), ^uintptr(0), uintptr(unsafe.Pointer(&tr)), dtCenter|dtVCenter|dtSingleLine|dtEndEllipsis)
	} else {
		label := title
		if muted {
			label += "  (muted)"
		}
		tr := rect{pad, s(10), osdW - pad - s(60), s(42)}
		procDrawTextW.Call(hdc, u16ptr(label), ^uintptr(0), uintptr(unsafe.Pointer(&tr)), dtLeft|dtVCenter|dtSingleLine|dtEndEllipsis)
		pr := rect{osdW - pad - s(70), s(10), osdW - pad, s(42)}
		procDrawTextW.Call(hdc, u16ptr(fmt.Sprintf("%d%%", p)), ^uintptr(0), uintptr(unsafe.Pointer(&pr)), dtRight|dtVCenter|dtSingleLine)
		bar := rect{pad, s(50), osdW - pad, s(58)}
		fill(bar, th.Border)
		w := int32(math.Round(float64(bar.Right-bar.Left) * float64(p) / 100))
		col := th.Accent
		if muted {
			col = th.Muted
		}
		if w > 0 {
			fill(rect{bar.Left, bar.Top, bar.Left + w, bar.Bottom}, col)
		}
	}
	procSelectObject.Call(hdc, old)
	procDeleteObject.Call(font)
}

// ---------- tray icon, drawn procedurally ----------

func makeIcon(size int) uintptr {
	px := make([]byte, size*size*4)
	sub := 4
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for sy := 0; sy < sub; sy++ {
				for sx := 0; sx < sub; sx++ {
					fx := (float64(x) + (float64(sx)+0.5)/float64(sub)) / float64(size)
					fy := (float64(y) + (float64(sy)+0.5)/float64(sub)) / float64(size)
					cr, cg, cb, ca := iconSample(fx, fy)
					r += cr * ca
					g += cg * ca
					b += cb * ca
					a += ca
				}
			}
			n := float64(sub * sub)
			alpha := a / n
			if a > 0 {
				r, g, b = r/a, g/a, b/a
			}
			// bottom-up rows, BGRA, premultiplication not required for icons
			row := size - 1 - y
			i := (row*size + x) * 4
			px[i+0] = byte(b * 255)
			px[i+1] = byte(g * 255)
			px[i+2] = byte(r * 255)
			px[i+3] = byte(alpha * 255)
		}
	}
	maskRow := ((size + 31) / 32) * 4
	buf := make([]byte, 40+len(px)+maskRow*size)
	binary.LittleEndian.PutUint32(buf[0:], 40)
	binary.LittleEndian.PutUint32(buf[4:], uint32(size))
	binary.LittleEndian.PutUint32(buf[8:], uint32(size*2))
	binary.LittleEndian.PutUint16(buf[12:], 1)
	binary.LittleEndian.PutUint16(buf[14:], 32)
	binary.LittleEndian.PutUint32(buf[20:], uint32(len(px)+maskRow*size))
	copy(buf[40:], px)
	h, _, _ := procCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 1, 0x00030000, uintptr(size), uintptr(size), 0)
	if h == 0 {
		h, _, _ = modUser32.NewProc("LoadIconW").Call(0, 32512)
	}
	return h
}

func roundRect(x, y, x0, y0, x1, y1, r float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	cx := math.Max(x0+r, math.Min(x, x1-r))
	cy := math.Max(y0+r, math.Min(y, y1-r))
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r
}

func iconSample(x, y float64) (r, g, b, a float64) {
	if !roundRect(x, y, 0.03, 0.03, 0.97, 0.97, 0.2) {
		return 0, 0, 0, 0
	}
	// knob
	dx, dy := x-0.7, y-0.5
	d := math.Sqrt(dx*dx + dy*dy)
	if d < 0.2 {
		if d < 0.13 && math.Abs(dx) < 0.03 && dy < 0 && dy > -0.12 {
			return 1, 1, 1, 1
		}
		return 0.34, 0.61, 1, 1
	}
	// 2x2 keys
	for _, k := range [][2]float64{{0.13, 0.2}, {0.32, 0.2}, {0.13, 0.52}, {0.32, 0.52}} {
		if roundRect(x, y, k[0], k[1], k[0]+0.15, k[1]+0.25, 0.04) {
			return 0.9, 0.92, 0.95, 1
		}
	}
	return 0.13, 0.15, 0.19, 1
}
