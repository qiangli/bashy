//go:build windows

package main

import (
	"os/exec"
	"syscall"
	"unsafe"
)

func configureChildProcess(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000 // Windows CREATE_NO_WINDOW
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

var user32 = syscall.NewLazyDLL("user32.dll")
var showWindow = user32.NewProc("ShowWindow")
var setForegroundWindow = user32.NewProc("SetForegroundWindow")

func focusWindow(p unsafe.Pointer) {
	showWindow.Call(uintptr(p), 9)
	setForegroundWindow.Call(uintptr(p))
}
func checkWebviewRuntime() error { return nil }
func checkTrayHost() error       { return nil }
