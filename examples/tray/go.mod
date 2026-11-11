module github.com/qiangli/bashy/examples/tray

go 1.27.2

require (
	fyne.io/systray v1.12.2
	github.com/godbus/dbus/v5 v5.1.0
	github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6
	gopkg.in/yaml.v3 v3.0.1
)

require golang.org/x/sys v0.15.0 // indirect

replace github.com/webview/webview_go => ./third_party/webview_go
