//go:build windows

// Package procutil holds small Windows-specific process-spawning helpers.
package procutil

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// HideConsole prevents a spawned console subprocess from flashing an extra
// console window on screen. It does not affect a program's own GUI window
// (e.g. scrcpy's or uxplay's mirrored video window).
func HideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
