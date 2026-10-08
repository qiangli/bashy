package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise the real publisher with cheap recording build/audit commands. An
// inherited lowercase tmp also reproduces the export collision on Unix; on
// Windows the shell inherits TMP's export attribute case-insensitively.
func TestArtifactPreservesTempEnvironment(t *testing.T) {
	shell := "/bin/sh"
	if runtime.GOOS == "windows" {
		var err error
		shell, err = exec.LookPath("bashy")
		if err != nil {
			t.Skip("requires bashy on PATH to execute the artifact script")
		}
	}
	script, err := os.ReadFile("../../scripts/build-bashy-artifact.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "publish", true: "failed-build-cleanup"}[fail], func(t *testing.T) {
			dir := t.TempDir()
			write := func(name, body string) {
				t.Helper()
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			write("artifact.sh", string(script))
			write("scripts/go-product.sh", `#!/bin/sh
set -eu
[ "$TMP" = "$EXPECTED_TEMP" ]
[ "$TEMP" = "$EXPECTED_TEMP" ]
[ "$GOTMPDIR" = "$EXPECTED_TEMP" ]
[ "$tmp" = "$EXPECTED_TEMP" ]
[ "$FAIL_BUILD" = no ] || exit 42
while [ "$1" != -o ]; do shift; done
printf artifact > "$2"
`)
			write("go", "#!/bin/sh\nexit 0\n")
			write("bin/product", "previous")
			value := "no"
			if fail {
				value = "yes"
			}
			cmd := exec.Command(shell, "artifact.sh", "bin/product", "", "test-tag")
			cmd.Dir = dir
			var env []string
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				switch strings.ToUpper(key) {
				case "TMP", "TEMP", "GOTMPDIR", "BASHY_EXE", "EXPECTED_TEMP", "FAIL_BUILD", "PATH":
				default:
					env = append(env, entry)
				}
			}
			// On Windows, use just TMP: the original bug exports an assignment to
			// lowercase tmp even though that name was never explicitly exported.
			env = append(env, "TMP="+dir, "TEMP="+dir, "GOTMPDIR="+dir, "EXPECTED_TEMP="+dir,
				"FAIL_BUILD="+value, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			if runtime.GOOS != "windows" {
				env = append(env, "tmp="+dir)
			}
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			if fail {
				if err == nil {
					t.Fatalf("expected build failure: %s", out)
				}
			} else if err != nil {
				t.Fatalf("artifact build: %v\n%s", err, out)
			}
			got, err := os.ReadFile(filepath.Join(dir, "bin", "product"))
			if err != nil {
				t.Fatal(err)
			}
			want := "artifact"
			if fail {
				want = "previous"
			}
			if string(got) != want {
				t.Fatalf("artifact = %q, want %q; output: %s", got, want, out)
			}
			pending, err := filepath.Glob(filepath.Join(dir, "bin", "product.pending.*"))
			if err != nil || len(pending) != 0 {
				t.Fatalf("pending files: %v, %v", pending, err)
			}
		})
	}
}
