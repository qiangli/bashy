package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"unsafe"

	webview "github.com/webview/webview_go"
)

func runWindow(raw, title string) error {
	if err := checkWebviewRuntime(); err != nil {
		return err
	}
	w := webview.New(false)
	if w == nil || w.Window() == unsafe.Pointer(nil) {
		return errors.New("embedded webview unavailable; install the OS webview runtime (WebView2 on Windows, WebKitGTK 4.1 on Linux)")
	}
	defer w.Destroy()
	w.SetTitle(title)
	w.SetSize(1100, 780, webview.HintNone)
	w.Navigate(raw)
	fmt.Println("READY")
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if sc.Text() == "focus" {
				w.Dispatch(func() { focusWindow(w.Window()) })
			}
		}
		w.Terminate()
	}()
	w.Run()
	return nil
}
