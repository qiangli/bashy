package agentos

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// adapterCacheEnvKey is the adapter scripts' cache root override. Every Go
// adapter builds its binary into "${BASHY_ADAPTER_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/bashy/adapters}"
// and hands that path to the native `go build -o`.
const adapterCacheEnvKey = "BASHY_ADAPTER_CACHE"

// adapterCacheExport returns the BASHY_ADAPTER_CACHE assignment the adapter
// process must see, or "" when the script's own default is correct.
//
// On Windows the shell exposes HOME in MSYS form (/c/Users/<user>) while
// go.exe reads paths natively: `-o /c/Users/...` lands under C:\c\Users\...
// and the script's `mv` then cannot find the build output, so every Go
// adapter fails on first use (rc.40 tier verdict 6c36c0a7, Windows lane).
// Export the root in drive form with forward slashes, which the shell, go.exe
// and the coreutils all read the same way; an operator's own override is
// kept, only respelled when it is in MSYS form.
func adapterCacheExport(env []string, windows bool, userHome string) string {
	if !windows {
		return ""
	}
	lookup := func(key string) string {
		prefix := key + "="
		for i := len(env) - 1; i >= 0; i-- {
			if strings.HasPrefix(env[i], prefix) {
				return strings.TrimSpace(env[i][len(prefix):])
			}
		}
		return ""
	}
	root := nativeWindowsPath(lookup(adapterCacheEnvKey))
	if root == "" {
		if xdg := nativeWindowsPath(lookup("XDG_CACHE_HOME")); xdg != "" {
			root = xdg + "/bashy/adapters"
		}
	}
	if root == "" {
		home := nativeWindowsPath(userHome)
		if home == "" {
			home = nativeWindowsPath(lookup("HOME"))
		}
		if home == "" {
			return ""
		}
		root = home + "/.cache/bashy/adapters"
	}
	return adapterCacheEnvKey + "=" + root
}

// nativeWindowsPath respells an MSYS drive path (/c/Users/x) as C:/Users/x
// and normalises separators to forward slashes; other spellings pass through.
func nativeWindowsPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if len(p) >= 2 && p[0] == '/' && isASCIILetter(p[1]) && (len(p) == 2 || p[2] == '/') {
		p = strings.ToUpper(p[1:2]) + ":" + p[2:]
	}
	return strings.TrimRight(p, "/")
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func hostAdapterCacheExport(env []string) string {
	home, _ := os.UserHomeDir()
	return adapterCacheExport(env, runtime.GOOS == "windows", filepath.Clean(home))
}
