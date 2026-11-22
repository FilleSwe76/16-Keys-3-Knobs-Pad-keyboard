//go:build windows

package main

import (
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var actionCh = make(chan Action, 128)

func repeatable(t string) bool {
	switch t {
	case "volume_up", "volume_down", "brightness_up", "brightness_down", "appvol_up", "appvol_down",
		"scroll_up", "scroll_down", "scroll_left", "scroll_right", "zoom_in", "zoom_out":
		return true
	}
	return false
}

func runActionAsync(a Action) {
	c := getSteps()
	switch a.Type {
	case "", "none":
		return
	case "brightness_up":
		brightnessAdd(c.br)
	case "brightness_down":
		brightnessAdd(-c.br)
	case "brightness_cycle":
		brightnessCycle()
	case "launch", "command":
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			defer recoverLog("launch")
			procCoInitializeEx.Call(0, 0x2|0x4) // apartment-threaded, no OLE1 DDE (for ShellExecute)
			runAction(a)
		}()
	default:
		select {
		case actionCh <- a:
		default:
		}
	}
}

type steps struct{ vol, br, app int }

func getSteps() steps {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return steps{cfg.VolumeStep, cfg.BrightnessStep, cfg.AppVolumeStep}
}

func actionWorker() {
	runtime.LockOSThread()
	procCoInitializeEx.Call(0, coinitMultithreaded)
	for a := range actionCh {
		func() {
			defer recoverLog("action " + a.Type)
			runAction(a)
		}()
	}
}

func runAction(a Action) {
	st := getSteps()
	switch a.Type {
	case "volume_up", "volume_down":
		d := st.vol
		if a.Type == "volume_down" {
			d = -d
		}
		if v, m, err := changeMasterVolume(eRender, d); err == nil {
			osdShow("Volume", v, m)
		} else {
			logf("volume: %v", err)
		}
	case "volume_mute":
		if v, m, err := toggleMute(eRender); err == nil {
			osdShow("Volume", v, m)
		}
	case "mic_mute":
		if _, m, err := toggleMute(eCapture); err == nil {
			if m {
				osdShow("Microphone muted", -1, true)
			} else {
				osdShow("Microphone on", -1, false)
			}
		} else {
			osdShow("No microphone found", -1, false)
		}
	case "appvol_up", "appvol_down", "appvol_mute":
		appVolumeAction(a.Type, st.app)
	case "media_playpause":
		tapKey(0xB3)
	case "media_next":
		tapKey(0xB0)
	case "media_prev":
		tapKey(0xB1)
	case "media_stop":
		tapKey(0xB2)
	case "shortcut":
		if vks, err := parseShortcut(a.Keys); err == nil {
			waitModifiersReleased()
			sendChord(vks)
		} else {
			logf("shortcut %q: %v", a.Keys, err)
		}
	case "text":
		waitModifiersReleased()
		typeText(a.Text)
	case "launch":
		launch(a.Path, a.Args)
	case "command":
		cmd := exec.Command("cmd.exe", "/C", a.Path)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if err := cmd.Start(); err != nil {
			logf("command: %v", err)
		} else {
			go cmd.Wait()
		}
	case "lock":
		procLockWorkStation.Call()
	case "scroll_up":
		sendWheel(120, false, false)
	case "scroll_down":
		sendWheel(-120, false, false)
	case "scroll_left":
		sendWheel(-120, true, false)
	case "scroll_right":
		sendWheel(120, true, false)
	case "zoom_in":
		sendWheel(120, false, true)
	case "zoom_out":
		sendWheel(-120, false, true)
	}
}

func launch(path, args string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	var argp uintptr
	if strings.TrimSpace(args) != "" {
		argp = u16ptr(args)
	}
	r, _, _ := procShellExecuteW.Call(0, u16ptr("open"), u16ptr(path), argp, 0, swShowNormal)
	if r <= 32 {
		logf("ShellExecute %q failed: %d", path, r)
		osdShow("Could not open "+path, -1, false)
	}
}

func waitModifiersReleased() {
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		if currentMods() == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func kbInput(vk, scan uint16, flags uint32) keyboardInput {
	return keyboardInput{Type: inputKeyboard, WVk: vk, WScan: scan, DwFlags: flags, DwExtraInfo: injectMagic}
}

func vkInput(vk uint16, up bool) keyboardInput {
	scan, _, _ := procMapVirtualKeyW.Call(uintptr(vk), 0)
	var f uint32
	if isExtendedVK(vk) {
		f |= keyeventfExtendedKey
	}
	if up {
		f |= keyeventfKeyUp
	}
	return kbInput(vk, uint16(scan), f)
}

func sendInputs(in []keyboardInput) {
	if len(in) == 0 {
		return
	}
	procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
}

func tapKey(vk uint16) { sendInputs([]keyboardInput{vkInput(vk, false), vkInput(vk, true)}) }

func sendChord(vks []uint16) {
	var in []keyboardInput
	for _, vk := range vks {
		in = append(in, vkInput(vk, false))
	}
	for i := len(vks) - 1; i >= 0; i-- {
		in = append(in, vkInput(vks[i], true))
	}
	sendInputs(in)
}

func typeText(s string) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var in []keyboardInput
	for _, r := range s {
		if r == '\n' {
			in = append(in, vkInput(vkReturn, false), vkInput(vkReturn, true))
			continue
		}
		if r == '\t' {
			in = append(in, vkInput(0x09, false), vkInput(0x09, true))
			continue
		}
		for _, u := range utf16.Encode([]rune{r}) {
			in = append(in, kbInput(0, u, keyeventfUnicode), kbInput(0, u, keyeventfUnicode|keyeventfKeyUp))
		}
	}
	// send in chunks so very long texts don't get dropped
	for len(in) > 0 {
		n := len(in)
		if n > 200 {
			n = 200
		}
		sendInputs(in[:n])
		in = in[n:]
		time.Sleep(5 * time.Millisecond)
	}
}

func sendWheel(delta int32, horizontal, withCtrl bool) {
	if withCtrl {
		sendInputs([]keyboardInput{vkInput(vkControl, false)})
	}
	flags := uint32(mouseeventfWheel)
	if horizontal {
		flags = mouseeventfHWheel
	}
	mi := mouseInput{Type: inputMouse, MouseData: uint32(delta), DwFlags: flags, DwExtraInfo: injectMagic}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&mi)), unsafe.Sizeof(mi))
	if withCtrl {
		sendInputs([]keyboardInput{vkInput(vkControl, true)})
	}
}
