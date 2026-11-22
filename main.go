//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
)

// version is set at build time with -ldflags "-X main.version=..."
var version = "1.0.0"

func logf(format string, a ...interface{}) { log.Printf(format, a...) }

func recoverLog(where string) {
	if r := recover(); r != nil {
		log.Printf("panic in %s: %v\n%s", where, r, debug.Stack())
	}
}

func openSettings() { showSettings() }

func main() {
	runtime.LockOSThread()
	background := false
	for _, a := range os.Args[1:] {
		if strings.EqualFold(a, "--background") {
			background = true
		}
	}

	appDir = filepath.Join(os.Getenv("APPDATA"), "PadDeck")
	_ = os.MkdirAll(appDir, 0755)
	logPath := filepath.Join(appDir, "paddeck.log")
	if st, err := os.Stat(logPath); err == nil && st.Size() > 2<<20 {
		_ = os.Rename(logPath, logPath+".old")
	}
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
		log.SetOutput(f)
	}
	// single instance: a second launch just brings up the running one's window
	h, _, e := procCreateMutexW.Call(0, 0, u16ptr("Local\\PadDeck.SingleInstance"))
	if h != 0 && e == syscall.ERROR_ALREADY_EXISTS {
		if !background {
			if w, _, _ := procFindWindowW.Call(u16ptr("PadDeckSettings"), 0); w != 0 {
				procPostMessageW.Call(w, wmAppShow, 0, 0)
			}
		}
		return
	}

	procSetProcessDPIAware.Call()
	logf("PadDeck %s starting", version)
	first := loadConfig()

	procCoInitializeEx.Call(0, 0x2|0x4) // STA for the GUI thread (file dialog, ShellExecute)
	enableVisualStyles()

	go actionWorker()
	go brightnessWorker()
	go dispatcher()
	go func() { // detect displays early so the first brightness turn is instant
		defer recoverLog("display warmup")
		brMu.Lock()
		ensureDisplays()
		brMu.Unlock()
	}()

	createWindows()
	createSettingsWindow()
	installHook()
	if hookHandle == 0 {
		procMessageBox("PadDeck could not install its keyboard hook, so the keypad won't work.\nSee "+logPath, "PadDeck")
	}
	if !background || first {
		openSettings()
	}

	messageLoop()

	uninstallHook()
	removeTrayIcon()
	logf("PadDeck exit")
}

func procMessageBox(text, title string) {
	modUser32.NewProc("MessageBoxW").Call(0, u16ptr(text), u16ptr(title), 0x30)
}

var _ = fmt.Sprintf
