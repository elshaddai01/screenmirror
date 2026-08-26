//go:build windows

// Package winapi wraps the small set of Win32 user32.dll calls needed to
// discover, retitle and position windows that a child process (uxplay.exe)
// opens on its own, and to read the primary monitor's resolution for tiling.
//
// Only stdlib syscall is used (no cgo, no external modules) so the project
// stays a single static Go binary with no network fetch needed to build.
package winapi

import (
	"syscall"
	"unsafe"
)

var (
	user32                        = syscall.NewLazyDLL("user32.dll")
	procEnumWindows               = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessId  = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible           = user32.NewProc("IsWindowVisible")
	procSetWindowTextW            = user32.NewProc("SetWindowTextW")
	procMoveWindow                = user32.NewProc("MoveWindow")
	procPostMessageW              = user32.NewProc("PostMessageW")
	procGetSystemMetrics          = user32.NewProc("GetSystemMetrics")
)

const (
	smCXScreen = 0
	smCYScreen = 1
	wmClose    = 0x0010
)

// ScreenSize returns the primary monitor's resolution, falling back to a
// sane default if the call ever fails.
func ScreenSize() (int, int) {
	w, _, _ := procGetSystemMetrics.Call(uintptr(smCXScreen))
	h, _, _ := procGetSystemMetrics.Call(uintptr(smCYScreen))
	if w == 0 || h == 0 {
		return 1920, 1080
	}
	return int(w), int(h)
}

// WindowsForPID returns the set of currently visible top-level window
// handles owned by the given process id.
func WindowsForPID(pid uint32) map[uintptr]bool {
	result := make(map[uintptr]bool)
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		if visible == 0 {
			return 1
		}
		var winPid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&winPid)))
		if winPid == pid {
			result[hwnd] = true
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return result
}

// SetTitle sets a window's title bar text.
func SetTitle(hwnd uintptr, title string) {
	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(ptr)))
}

// Move repositions and resizes a window.
func Move(hwnd uintptr, x, y, w, h int) {
	procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 1)
}

// Close asks a window to close itself (WM_CLOSE), same as clicking its X button.
func Close(hwnd uintptr) {
	procPostMessageW.Call(hwnd, uintptr(wmClose), 0, 0)
}
