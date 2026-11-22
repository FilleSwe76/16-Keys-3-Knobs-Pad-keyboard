//go:build windows

package main

import (
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const injectMagic = 0x50414444 // marks our own SendInput events

type hookEvent struct {
	slot   string
	repeat bool
}

var (
	hookHandle uintptr
	bindings   atomic.Pointer[map[uint32]string]
	learnArmed atomic.Bool
	learnCh    = make(chan uint32, 1)
	paused     atomic.Bool
	eventCh    = make(chan hookEvent, 256)
	swallowed  [256]bool // only touched on the hook thread
	hookCB     = syscall.NewCallback(hookProc)

	firedMu   sync.Mutex
	firedSeq  int64
	firedSlot string
	firedCode string
	firedAt   time.Time
)

func rebuildBindings() {
	cfgMu.RLock()
	m := make(map[uint32]string)
	for id, s := range cfg.Slots {
		if s.Trigger == "" {
			continue
		}
		if code, err := parseTrigger(s.Trigger); err == nil {
			m[code] = id
		}
	}
	cfgMu.RUnlock()
	bindings.Store(&m)
}

func asyncDown(vk int) bool {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}

func currentMods() uint32 {
	var m uint32
	if asyncDown(vkControl) {
		m |= modCtrl
	}
	if asyncDown(vkShift) {
		m |= modShift
	}
	if asyncDown(vkMenu) {
		m |= modAlt
	}
	if asyncDown(vkLWin) || asyncDown(vkRWin) {
		m |= modWin
	}
	return m
}

func recordFired(slot, code string) {
	firedMu.Lock()
	firedSeq++
	firedSlot, firedCode, firedAt = slot, code, time.Now()
	firedMu.Unlock()
	notifyFired()
}

func hookProc(nCode int, wParam, lParam uintptr) uintptr {
	if nCode == 0 {
		k := (*kbdllHookStruct)(unsafe.Pointer(lParam))
		if k.DwExtraInfo != injectMagic && k.VkCode < 256 && !isModifierVK(k.VkCode) {
			vk := k.VkCode
			if k.Flags&llkhfUp != 0 {
				if swallowed[vk] {
					swallowed[vk] = false
					return 1
				}
			} else {
				repeat := swallowed[vk]
				code := vk | currentMods()<<16
				if learnArmed.Load() {
					learnArmed.Store(false)
					select {
					case learnCh <- code:
					default:
					}
					swallowed[vk] = true
					return 1
				}
				if !paused.Load() {
					if m := bindings.Load(); m != nil {
						if slot, ok := (*m)[code]; ok {
							swallowed[vk] = true
							select {
							case eventCh <- hookEvent{slot, repeat}:
							default:
							}
							return 1
						}
					}
				}
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return r
}

func installHook() {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	h, _, err := procSetWindowsHookExW.Call(whKeyboardLL, hookCB, hInst, 0)
	if h == 0 {
		logf("SetWindowsHookEx failed: %v", err)
		return
	}
	hookHandle = h
}

func uninstallHook() {
	if hookHandle != 0 {
		procUnhookWindowsHookEx.Call(hookHandle)
		hookHandle = 0
	}
}

func dispatcher() {
	defer recoverLog("dispatcher")
	for ev := range eventCh {
		cfgMu.RLock()
		var a Action
		trig := ""
		if s := cfg.Slots[ev.slot]; s != nil {
			a = s.Action
			trig = s.Trigger
		}
		cfgMu.RUnlock()
		recordFired(ev.slot, trig)
		if ev.repeat && !repeatable(a.Type) {
			continue
		}
		runActionAsync(a)
	}
}

// learn waits for the next key press (from any keyboard) and returns its code.
func learn(cancel <-chan struct{}, timeout time.Duration) (uint32, bool) {
	select {
	case <-learnCh:
	default:
	}
	learnArmed.Store(true)
	defer learnArmed.Store(false)
	select {
	case c := <-learnCh:
		return c, true
	case <-cancel:
		return 0, false
	case <-time.After(timeout):
		return 0, false
	}
}
