//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var showWindow = user32.NewProc("ShowWindow")
var setForegroundWindow = user32.NewProc("SetForegroundWindow")

func focusWindow(p unsafe.Pointer) {
	showWindow.Call(uintptr(p), 9)
	setForegroundWindow.Call(uintptr(p))
}
func checkWebviewRuntime() error { return nil }
func checkTrayHost() error       { return nil }
