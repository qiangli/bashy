//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#include <Cocoa/Cocoa.h>
static void focus_native(void *p) {
 NSWindow *w = (__bridge NSWindow *)p;
 [NSApp activateIgnoringOtherApps:YES];
 [w makeKeyAndOrderFront:nil];
}
*/
import "C"
import "unsafe"

func focusWindow(p unsafe.Pointer) { C.focus_native(p) }
func checkWebviewRuntime() error   { return nil }
func checkTrayHost() error         { return nil }
