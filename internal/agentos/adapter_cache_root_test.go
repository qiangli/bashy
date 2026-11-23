package agentos

import "testing"

// The adapter scripts derive their cache root from HOME, which the shell
// exposes in MSYS form on Windows; go.exe would root /c/Users/... on the
// current drive and the first-use build would vanish (rc.40 Windows tier FAIL).
func TestAdapterCacheExportWindowsUsesDriveForm(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		home string
		want string
	}{
		{"home from the OS", []string{"HOME=/c/Users/x"}, `C:\Users\x`, "BASHY_ADAPTER_CACHE=C:/Users/x/.cache/bashy/adapters"},
		{"home only in MSYS form", []string{"HOME=/c/Users/x"}, "", "BASHY_ADAPTER_CACHE=C:/Users/x/.cache/bashy/adapters"},
		{"xdg cache wins over home", []string{"HOME=/c/Users/x", "XDG_CACHE_HOME=/d/cache"}, `C:\Users\x`, "BASHY_ADAPTER_CACHE=D:/cache/bashy/adapters"},
		{"override in msys form is respelled", []string{"BASHY_ADAPTER_CACHE=/c/Users/x/.cache/bashy/adapters"}, `C:\Users\x`, "BASHY_ADAPTER_CACHE=C:/Users/x/.cache/bashy/adapters"},
		{"native override kept", []string{`BASHY_ADAPTER_CACHE=D:\adapters\`}, `C:\Users\x`, "BASHY_ADAPTER_CACHE=D:/adapters"},
		{"no home at all", nil, "", ""},
	}
	for _, tc := range cases {
		if got := adapterCacheExport(tc.env, true, tc.home); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
	if got := adapterCacheExport([]string{"HOME=/c/Users/x"}, false, "/c/Users/x"); got != "" {
		t.Errorf("non-windows host must leave the script default alone, got %q", got)
	}
}

func TestAppendAdapterEnvExportsCacheOnWindowsOnly(t *testing.T) {
	env := appendAdapterEnv([]string{"PATH=/usr/bin", "HOME=/c/Users/x"}, "mktemp", true)
	count := 0
	for _, kv := range env {
		if len(kv) > len(adapterCacheEnvKey) && kv[:len(adapterCacheEnvKey)+1] == adapterCacheEnvKey+"=" {
			count++
			if hostAdapterCacheExport(nil) == "" {
				t.Fatalf("non-windows host exported %q", kv)
			}
		}
	}
	if count > 1 {
		t.Fatalf("duplicate %s in adapter env: %q", adapterCacheEnvKey, env)
	}
}
