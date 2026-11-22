//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")
	modGdi32    = syscall.NewLazyDLL("gdi32.dll")
	modShell32  = syscall.NewLazyDLL("shell32.dll")
	modOle32    = syscall.NewLazyDLL("ole32.dll")
	modDxva2    = syscall.NewLazyDLL("dxva2.dll")
	modAdvapi32 = syscall.NewLazyDLL("advapi32.dll")
	modSetupapi = syscall.NewLazyDLL("setupapi.dll")
	modHid      = syscall.NewLazyDLL("hid.dll")

	procSetWindowsHookExW          = modUser32.NewProc("SetWindowsHookExW")
	procCallNextHookEx             = modUser32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx        = modUser32.NewProc("UnhookWindowsHookEx")
	procGetMessageW                = modUser32.NewProc("GetMessageW")
	procTranslateMessage           = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW           = modUser32.NewProc("DispatchMessageW")
	procPostMessageW               = modUser32.NewProc("PostMessageW")
	procPostQuitMessage            = modUser32.NewProc("PostQuitMessage")
	procDefWindowProcW             = modUser32.NewProc("DefWindowProcW")
	procRegisterClassExW           = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW            = modUser32.NewProc("CreateWindowExW")
	procDestroyWindow              = modUser32.NewProc("DestroyWindow")
	procShowWindow                 = modUser32.NewProc("ShowWindow")
	procSetWindowPos               = modUser32.NewProc("SetWindowPos")
	procInvalidateRect             = modUser32.NewProc("InvalidateRect")
	procBeginPaint                 = modUser32.NewProc("BeginPaint")
	procEndPaint                   = modUser32.NewProc("EndPaint")
	procFillRect                   = modUser32.NewProc("FillRect")
	procDrawTextW                  = modUser32.NewProc("DrawTextW")
	procSetTimer                   = modUser32.NewProc("SetTimer")
	procKillTimer                  = modUser32.NewProc("KillTimer")
	procGetAsyncKeyState           = modUser32.NewProc("GetAsyncKeyState")
	procSendInput                  = modUser32.NewProc("SendInput")
	procMapVirtualKeyW             = modUser32.NewProc("MapVirtualKeyW")
	procGetForegroundWindow        = modUser32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId   = modUser32.NewProc("GetWindowThreadProcessId")
	procEnumChildWindows           = modUser32.NewProc("EnumChildWindows")
	procLockWorkStation            = modUser32.NewProc("LockWorkStation")
	procCreatePopupMenu            = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW                = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu             = modUser32.NewProc("TrackPopupMenu")
	procDestroyMenu                = modUser32.NewProc("DestroyMenu")
	procSetForegroundWindow        = modUser32.NewProc("SetForegroundWindow")
	procGetCursorPos               = modUser32.NewProc("GetCursorPos")
	procRegisterWindowMessageW     = modUser32.NewProc("RegisterWindowMessageW")
	procSetProcessDPIAware         = modUser32.NewProc("SetProcessDPIAware")
	procGetDpiForSystem            = modUser32.NewProc("GetDpiForSystem")
	procGetSystemMetrics           = modUser32.NewProc("GetSystemMetrics")
	procSetLayeredWindowAttributes = modUser32.NewProc("SetLayeredWindowAttributes")
	procSetWindowRgn               = modUser32.NewProc("SetWindowRgn")
	procEnumDisplayMonitors        = modUser32.NewProc("EnumDisplayMonitors")
	procCreateIconFromResourceEx   = modUser32.NewProc("CreateIconFromResourceEx")
	procLoadCursorW                = modUser32.NewProc("LoadCursorW")

	procCreateSolidBrush   = modGdi32.NewProc("CreateSolidBrush")
	procDeleteObject       = modGdi32.NewProc("DeleteObject")
	procSetBkMode          = modGdi32.NewProc("SetBkMode")
	procSetTextColor       = modGdi32.NewProc("SetTextColor")
	procCreateFontW        = modGdi32.NewProc("CreateFontW")
	procSelectObject       = modGdi32.NewProc("SelectObject")
	procCreateRoundRectRgn = modGdi32.NewProc("CreateRoundRectRgn")

	procGetModuleHandleW           = modKernel32.NewProc("GetModuleHandleW")
	procCreateMutexW               = modKernel32.NewProc("CreateMutexW")
	procOpenProcess                = modKernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = modKernel32.NewProc("QueryFullProcessImageNameW")

	procShellNotifyIconW = modShell32.NewProc("Shell_NotifyIconW")
	procShellExecuteW    = modShell32.NewProc("ShellExecuteW")

	procCoInitializeEx   = modOle32.NewProc("CoInitializeEx")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")

	procGetNumberOfPhysicalMonitorsFromHMONITOR = modDxva2.NewProc("GetNumberOfPhysicalMonitorsFromHMONITOR")
	procGetPhysicalMonitorsFromHMONITOR         = modDxva2.NewProc("GetPhysicalMonitorsFromHMONITOR")
	procDestroyPhysicalMonitor                  = modDxva2.NewProc("DestroyPhysicalMonitor")
	procGetVCPFeatureAndVCPFeatureReply         = modDxva2.NewProc("GetVCPFeatureAndVCPFeatureReply")
	procSetVCPFeature                           = modDxva2.NewProc("SetVCPFeature")

	procRegSetValueExW  = modAdvapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW = modAdvapi32.NewProc("RegDeleteValueW")

	procSetupDiGetClassDevsW             = modSetupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces      = modSetupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW = modSetupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList     = modSetupapi.NewProc("SetupDiDestroyDeviceInfoList")

	procHidDGetHidGuid        = modHid.NewProc("HidD_GetHidGuid")
	procHidDGetAttributes     = modHid.NewProc("HidD_GetAttributes")
	procHidDGetPreparsedData  = modHid.NewProc("HidD_GetPreparsedData")
	procHidDFreePreparsedData = modHid.NewProc("HidD_FreePreparsedData")
	procHidPGetCaps           = modHid.NewProc("HidP_GetCaps")
	procHidDSetOutputReport   = modHid.NewProc("HidD_SetOutputReport")
)

const (
	whKeyboardLL  = 13
	wmKeydown     = 0x0100
	wmDestroy     = 0x0002
	wmPaint       = 0x000F
	wmTimer       = 0x0113
	wmNull        = 0x0000
	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmContextMenu = 0x007B
	wmApp         = 0x8000
	wmTray        = wmApp + 1
	wmAppOSD      = wmApp + 2
	wmAppQuit     = wmApp + 3

	llkhfUp = 0x80

	wsOverlapped    = 0x00000000
	wsPopup         = 0x80000000
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExLayered     = 0x00080000
	wsExNoActivate  = 0x08000000
	wsExTransparent = 0x00000020

	swHide           = 0
	swShowNoActivate = 4
	swShowNormal     = 1

	swpNoActivate = 0x0010
	swpShowWindow = 0x0040

	inputMouse    = 0
	inputKeyboard = 1

	keyeventfExtendedKey = 0x0001
	keyeventfKeyUp       = 0x0002
	keyeventfUnicode     = 0x0004

	mouseeventfWheel  = 0x0800
	mouseeventfHWheel = 0x1000

	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4

	mfString       = 0x0
	mfSeparator    = 0x800
	mfChecked      = 0x8
	tpmReturnCmd   = 0x0100
	tpmRightButton = 0x0002

	dtLeft        = 0x0
	dtCenter      = 0x1
	dtRight       = 0x2
	dtVCenter     = 0x4
	dtSingleLine  = 0x20
	dtEndEllipsis = 0x8000

	coinitMultithreaded = 0x0
	clsctxAll           = 0x17
)

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

type msgT struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type kbdllHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type keyboardInput struct {
	Type        uint32
	_           uint32
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	_           uint32
	DwExtraInfo uintptr
	_           [8]byte
}

type mouseInput struct {
	Type        uint32
	_           uint32
	Dx, Dy      int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	_           uint32
	DwExtraInfo uintptr
}

type paintStruct struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     rect
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         guid
	HBalloonIcon     uintptr
}

func u16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	return p
}

func u16ptr(s string) uintptr { return uintptr(unsafe.Pointer(u16(s))) }

func rgb(r, g, b uint8) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

func parseGUID(s string) guid {
	// "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
	hex := func(str string) uint64 {
		var v uint64
		for _, c := range str {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= uint64(c - '0')
			case c >= 'a' && c <= 'f':
				v |= uint64(c-'a') + 10
			case c >= 'A' && c <= 'F':
				v |= uint64(c-'A') + 10
			}
		}
		return v
	}
	var g guid
	g.Data1 = uint32(hex(s[0:8]))
	g.Data2 = uint16(hex(s[9:13]))
	g.Data3 = uint16(hex(s[14:18]))
	g.Data4[0] = byte(hex(s[19:21]))
	g.Data4[1] = byte(hex(s[21:23]))
	for i := 0; i < 6; i++ {
		g.Data4[2+i] = byte(hex(s[24+i*2 : 26+i*2]))
	}
	return g
}

// vt returns the i-th method pointer of a COM object's vtable.
func vt(obj uintptr, i int) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	return *(*uintptr)(unsafe.Pointer(vtbl + uintptr(i)*unsafe.Sizeof(uintptr(0))))
}

func comRelease(obj uintptr) {
	if obj != 0 {
		syscall.SyscallN(vt(obj, 2), obj)
	}
}

func failed(hr uintptr) bool { return int32(hr) < 0 }
