package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Run the real bootstrap with a recording product builder: neither managed nor
// PATH Go may bypass it. The runtime itself is covered by the process probes.
func TestRootBootstrapUsesProductBuilder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap")
	}
	bootstrap, err := os.ReadFile("../../bashy")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"plain", "BASHY", "BASHY_EXE", "PATH"} {
		t.Run(mode, func(t *testing.T) {
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
			write("bashy", string(bootstrap))
			write("go.mod", "module github.com/qiangli/bashy\n\nreplace mvdan.cc/sh/v3 => github.com/qiangli/sh/v3 test-pin\n")
			write("scripts/build-meet-spa.sh", "#!/bin/sh\nprintf test-spa\n")
			write("path/git", "#!/bin/sh\nprintf test-time\n")
			write("path/go", "#!/bin/sh\necho bypassed-product-builder >&2\nexit 92\n")
			write("managed", "#!/bin/sh\necho bypassed-product-builder >&2\nexit 93\n")
			write("scripts/go-product.sh", `#!/bin/sh
set -eu
[ "${BASHY_EXE-}" = "$EXPECTED_FRONT" ]
[ "$1" = build ]
shift
[ "$1" = -trimpath ]
shift
[ "$1" = -tags=test-spa ]
shift
[ "$1" = -ldflags ]
shift
case "$1" in *ShellRuntimeCommit=test-pin*) ;; *) exit 94 ;; esac
shift
[ "$1" = -o ]
output=$2
shift 2
[ "$#" = 1 ] && [ "$1" = ./cmd/bashy ]
printf '#!/bin/sh\nprintf "product:%%s:%%s" "$1" "$2"\n' > "$output"
chmod +x "$output"
`)
			for _, name := range []string{"dirname", "mkdir", "sed", "chmod"} {
				path, err := exec.LookPath(name)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path, filepath.Join(dir, "path", name)); err != nil {
					t.Fatal(err)
				}
			}
			var env []string
			for _, entry := range os.Environ() {
				key := strings.SplitN(entry, "=", 2)[0]
				switch key {
				case "BASHY", "BASHY_EXE", "PATH", "BASHY_BOOTSTRAP_REBUILD", "EXPECTED_FRONT":
				default:
					env = append(env, entry)
				}
			}
			front := ""
			switch mode {
			case "BASHY":
				front = filepath.Join(dir, "managed")
				env = append(env, "BASHY="+front)
			case "BASHY_EXE":
				front = filepath.Join(dir, "managed")
				env = append(env, "BASHY_EXE="+front, "BASHY=/must-not-win")
			case "PATH":
				front = filepath.Join(dir, "path", "bashy")
				if err := os.Symlink(filepath.Join(dir, "managed"), front); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command(filepath.Join(dir, "bashy"), "arg with spaces", "last")
			cmd.Env = append(env, "PATH="+filepath.Join(dir, "path"), "EXPECTED_FRONT="+front)
			out, err := cmd.CombinedOutput()
			if err != nil || !strings.HasSuffix(string(out), "product:arg with spaces:last") {
				t.Fatalf("bootstrap: %v\n%s", err, out)
			}
		})
	}
}
