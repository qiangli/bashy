package main

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Autostart entry names: LaunchAgent label, Run value name, .desktop file.
const (
	launchAgentLabel = "com.qiangli.bashy-tray"
	runValueName     = "BashyTray"
	runKey           = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	desktopFileName  = "bashy-tray.desktop"
)

// autostartTarget is where the login entry lives. For macOS and Linux it is a
// file; for Windows it is the registry value (Path is empty).
type autostartTarget struct {
	Path    string
	Content string
}

func xdgConfigDir(home string) string {
	if d := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); d != "" {
		return d
	}
	return filepath.Join(home, ".config")
}

func plistEscape(s string) string { return html.EscapeString(s) }

func desktopQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"'\\<>~|&;$*?#()`") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`)
	return `"` + r.Replace(s) + `"`
}

// autostartFor computes the login entry for goos launching exe.
func autostartFor(goos, home, exe string) (autostartTarget, error) {
	switch goos {
	case "darwin":
		return autostartTarget{
			Path: filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"),
			Content: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + launchAgentLabel + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + plistEscape(exe) + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
</dict>
</plist>
`}, nil
	case "linux":
		return autostartTarget{
			Path: filepath.Join(xdgConfigDir(home), "autostart", desktopFileName),
			Content: `[Desktop Entry]
Type=Application
Name=Bashy Tray
Comment=Bashy and outpost status in the system tray
Exec=` + desktopQuote(exe) + `
Terminal=false
X-GNOME-Autostart-enabled=true
`}, nil
	case "windows":
		return autostartTarget{Content: `"` + exe + `"`}, nil
	}
	return autostartTarget{}, fmt.Errorf("autostart is not supported on %s", goos)
}

// regRunner runs reg.exe; replaced in tests.
var regRunner = func(args ...string) ([]byte, error) {
	cmd := exec.Command("reg", args...)
	configureChildProcess(cmd)
	return cmd.CombinedOutput()
}

// autostartInstall writes the login entry; it reports whether anything changed.
func autostartInstall(goos, home, exe string) (bool, error) {
	t, err := autostartFor(goos, home, exe)
	if err != nil {
		return false, err
	}
	if goos == "windows" {
		if cur, ok := regQuery(); ok && cur == t.Content {
			return false, nil
		}
		if out, err := regRunner("add", runKey, "/v", runValueName, "/t", "REG_SZ", "/d", t.Content, "/f"); err != nil {
			return false, fmt.Errorf("reg add: %v: %s", err, bytes.TrimSpace(out))
		}
		return true, nil
	}
	if old, err := os.ReadFile(t.Path); err == nil && string(old) == t.Content {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(t.Path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(t.Path, []byte(t.Content), 0o644)
}

// autostartUninstall removes the login entry; it reports whether one existed.
func autostartUninstall(goos, home string) (bool, error) {
	t, err := autostartFor(goos, home, "")
	if err != nil {
		return false, err
	}
	if goos == "windows" {
		if _, ok := regQuery(); !ok {
			return false, nil
		}
		if out, err := regRunner("delete", runKey, "/v", runValueName, "/f"); err != nil {
			return false, fmt.Errorf("reg delete: %v: %s", err, bytes.TrimSpace(out))
		}
		return true, nil
	}
	if err := os.Remove(t.Path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// regQuery returns the current Run value data, if the value exists.
func regQuery() (string, bool) {
	out, err := regRunner("query", runKey, "/v", runValueName)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.SplitN(strings.TrimSpace(line), "REG_SZ", 2)
		if len(f) == 2 && strings.TrimSpace(f[0]) == runValueName {
			return strings.TrimSpace(f[1]), true
		}
	}
	return "", false
}

// autostartCommand implements `bashy-tray install|uninstall`.
func autostartCommand(verb string, stdout func(string)) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if verb == "uninstall" {
		changed, err := autostartUninstall(runtime.GOOS, home)
		if err != nil {
			return err
		}
		if changed {
			stdout("bashy-tray: autostart removed")
		} else {
			stdout("bashy-tray: autostart not installed; nothing to do")
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	changed, err := autostartInstall(runtime.GOOS, home, exe)
	if err != nil {
		return err
	}
	if changed {
		stdout("bashy-tray: autostart installed for " + exe)
	} else {
		stdout("bashy-tray: autostart already installed; nothing to do")
	}
	return nil
}
