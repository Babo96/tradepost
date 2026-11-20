//go:build windows

// Package tray provides a small Windows notification-area icon for the
// companion. It uses only the Win32 API via syscall, so CGO_ENABLED=0 builds
// still work.
package tray

import (
	"context"
	"log"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/babo96/tradepost/companion/internal/app"
	"github.com/babo96/tradepost/companion/internal/browser"
)

const (
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmContextMenu   = 0x007B
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmLButtonUp     = 0x0202
	wmNull          = 0x0000

	trayCallbackMessage = 0x8000 + 1
	trayUID             = 1
	nimAdd              = 0
	nimDelete           = 2
	nifMessage          = 0x00000001
	nifIcon             = 0x00000002
	nifTip              = 0x00000004
	mfString            = 0x00000000
	tpmRightButton      = 0x0002
	tpmReturnCmd        = 0x0100
	imageIcon           = 1
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procShellNotifyIconW    = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")

	trayWindow uintptr
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     syscall.Handle
}

type point struct {
	X int32
	Y int32
}

type msg struct {
	Hwnd     syscall.Handle
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type notifyIconData struct {
	CbSize           uint32
	HWnd             syscall.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            syscall.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     syscall.Handle
}

var (
	trayApp  *app.App
	trayURL  string
	trayQuit func()
	trayNID  notifyIconData
)

// Start creates the tray icon on a dedicated OS thread. onQuit is called after
// the tray window is destroyed.
func Start(application *app.App, dashboardURL string, onQuit func()) {
	trayApp = application
	trayURL = dashboardURL
	trayQuit = onQuit
	go func() {
		runtime.LockOSThread()
		run()
	}()
}

// Stop asks the tray thread to close. Safe to call when no tray exists.
func Stop() {
	hwnd := atomic.LoadUintptr(&trayWindow)
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
}

func run() {
	instance, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TradePostTrayWindow")
	class := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   syscall.NewCallback(windowProc),
		Instance:  syscall.Handle(instance),
		ClassName: className,
	}
	if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		log.Printf("tray: RegisterClassEx failed: %v", err)
		return
	}
	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		0,
		0, 0, 0, 0,
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		log.Printf("tray: CreateWindowEx failed: %v", err)
		return
	}
	atomic.StoreUintptr(&trayWindow, hwnd)

	icon, _, _ := procLoadIconW.Call(instance, 1) // icon group 1 from rsrc_windows_amd64.syso
	if icon == 0 {
		icon, _, _ = procLoadIconW.Call(0, 32512) // IDI_APPLICATION
	}
	trayNID = notifyIconData{
		HWnd:             syscall.Handle(hwnd),
		UID:              trayUID,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: trayCallbackMessage,
		HIcon:            syscall.Handle(icon),
	}
	trayNID.CbSize = uint32(unsafe.Sizeof(trayNID))
	tip, _ := syscall.UTF16FromString("TradePost Companion")
	copy(trayNID.SzTip[:], tip)
	if ok, _, err := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&trayNID))); ok == 0 {
		log.Printf("tray: Shell_NotifyIcon add failed: %v", err)
		procDestroyWindow.Call(hwnd)
		return
	}

	var message msg
	for {
		ret, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(ret) == -1 {
			log.Printf("tray: GetMessage failed: %v", err)
			break
		}
		if ret == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
}

func windowProc(hwnd syscall.Handle, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case trayCallbackMessage:
		switch lParam {
		case wmRButtonUp, wmContextMenu:
			showMenu(hwnd)
		case wmLButtonUp, wmLButtonDblClk:
			openDashboard()
		}
		return 0
	case wmClose:
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	case wmDestroy:
		if trayNID.CbSize != 0 {
			trayNID.UFlags = 0
			procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&trayNID)))
			trayNID.CbSize = 0
		}
		atomic.StoreUintptr(&trayWindow, 0)
		procPostQuitMessage.Call(0)
		if trayQuit != nil {
			trayQuit()
		}
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return ret
}

func showMenu(hwnd syscall.Handle) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	open, _ := syscall.UTF16PtrFromString("Open dashboard")
	syncLabel, _ := syscall.UTF16PtrFromString("Sync now")
	quit, _ := syscall.UTF16PtrFromString("Quit")
	procAppendMenuW.Call(menu, mfString, 1, uintptr(unsafe.Pointer(open)))
	procAppendMenuW.Call(menu, mfString, 2, uintptr(unsafe.Pointer(syncLabel)))
	procAppendMenuW.Call(menu, mfString, 3, uintptr(unsafe.Pointer(quit)))

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(uintptr(hwnd))
	cmd, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmRightButton|tpmReturnCmd,
		uintptr(pt.X), uintptr(pt.Y),
		0, uintptr(hwnd), 0,
	)
	procPostMessageW.Call(uintptr(hwnd), wmNull, 0, 0)
	switch cmd {
	case 1:
		openDashboard()
	case 2:
		go syncNow()
	case 3:
		procDestroyWindow.Call(uintptr(hwnd))
	}
}

func openDashboard() {
	if trayURL == "" {
		return
	}
	if err := browser.Open(trayURL); err != nil {
		log.Printf("tray: open dashboard: %v", err)
	}
}

func syncNow() {
	if trayApp == nil {
		return
	}
	if err := trayApp.Sync(context.Background()); err != nil {
		log.Printf("tray: sync: %v", err)
	}
}
