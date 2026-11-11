//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
static void focus_native(void *p) { gtk_window_present(GTK_WINDOW(p)); }
*/
import "C"
import (
	"errors"
	"github.com/godbus/dbus/v5"
	"os"
	"os/exec"
	"unsafe"
)

func focusWindow(p unsafe.Pointer) { C.focus_native(p) }
func checkWebviewRuntime() error {
	if err := exec.Command("pkg-config", "--exists", "webkit2gtk-4.1").Run(); err != nil {
		return errors.New("WebKitGTK 4.1 runtime/development package missing (webkit2gtk-4.1)")
	}
	return nil
}
func checkTrayHost() error {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return errors.New("no graphical desktop session for tray")
	}
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		return errors.New("no desktop session bus for tray; enable GNOME AppIndicator/StatusNotifier support")
	}
	defer bus.Close()
	var owner bool
	err = bus.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&owner)
	if err != nil || !owner {
		return errors.New("no StatusNotifier/AppIndicator tray host; enable the GNOME AppIndicator extension")
	}
	return nil
}
