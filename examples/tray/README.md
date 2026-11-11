# Bashy tray example

`bashy-tray` is an optional desktop helper built from its own Go module. It is
not imported by or linked into the core `bashy` binary. The tray uses
`fyne.io/systray`; every Admin, Apps, or custom window is a separate
`bashy-tray window URL TITLE` process using `webview_go` and the platform's
embedded engine. Clicking an already open item focuses that window. No action
opens a system browser.

The menu shows a status line and colored icon: green when outpost is running
and paired, amber when running without a current pairing, gray when off. It
polls `outpost status --local-presence` and `outpost status --json` every ten
seconds. Admin uses `http://127.0.0.1:17777`, or the host:port in
`OUTPOST_ADMIN_ADDR`, and is disabled when outpost is off. Apps uses
`http://127.0.0.1:22749/`. When that address does not answer, the tray starts
`bashy app serve` and owns that process. Quit closes its windows and app server
(if owned), but never stops outpost. Start calls `outpost enable`, which clears
the stop intent and starts outpost through its supervisor or detached fallback;
Stop calls `outpost stop`.

Place an optional `tray.yaml` in your bashy config directory
(`$XDG_CONFIG_HOME/bashy` when set, otherwise `~/.config/bashy`):

```yaml
items:
  - label: Local dashboard
    url: http://127.0.0.1:9000/
```

Each item opens in an embedded webview. URLs must be absolute HTTP or HTTPS
without credentials. The tray reads this file at startup; restart it after
editing. Keep `bashy` and `outpost` on `PATH`.

## Build and run

All builds use Go with cgo enabled. Build from `examples/tray`:

- **macOS:** Install Xcode Command Line Tools, then run
  `CGO_ENABLED=1 go build -o bashy-tray .` and `./bashy-tray` in a logged-in
  desktop session. WKWebView ships with macOS.
- **Windows:** Install Go and a C/C++ toolchain suitable for cgo, such as
  MSYS2 MinGW-w64. In PowerShell run `$env:CGO_ENABLED='1'; go build -ldflags
  '-H=windowsgui' -o bashy-tray.exe .; .\bashy-tray.exe`. Install Microsoft
  WebView2 Evergreen Runtime if absent. The GUI build and its child processes
  do not open console windows.
- **Linux:** Install Go, a C/C++ compiler, `pkg-config`, GTK 3 development
  headers, WebKitGTK **4.1** development/runtime packages, and a desktop tray
  host. On Debian or Ubuntu the packages include `libgtk-3-dev` and
  `libwebkit2gtk-4.1-dev`. On stock GNOME, enable the AppIndicator and
  KStatusNotifierItem extension. Run `CGO_ENABLED=1 go build -o bashy-tray .`
  and `./bashy-tray` inside the graphical session.

The Linux build uses a local copy of `webview_go` because the upstream Go
binding pins WebKitGTK 4.0 in its cgo flags. The copy changes that flag to 4.1
and returns `nil` when native window construction fails, allowing a clear
runtime error. Source licenses are retained under `third_party/webview_go`.

The Windows build uses a local copy of `fyne.io/systray` v1.12.2 (Apache-2.0,
license retained under `third_party/systray`). Its one change: the tray menu
opens with `TPM_NOANIMATION` and posts `WM_NULL` after `TrackPopupMenu`. On a
Windows 11 test host the menu fade-in left the items unpainted until the
pointer moved over them.
A missing Linux tray host or WebKitGTK package, or a missing Windows WebView2
runtime, is an error; there is no browser fallback.

## Verification

`bashy-tray install` / `uninstall` add or remove login autostart (see the kb runbook page on the tray).

Run `go test ./...` and `go build .` in this directory. On each desktop, run
and capture screenshots showing the three icon states, Admin and Apps windows,
a `tray.yaml` item, and the tray after login. Check that Quit leaves a
previously running outpost up and stops only an app server launched by the
tray. The CLI can test a single window with
`bashy-tray window http://127.0.0.1:22749/ Apps`.
