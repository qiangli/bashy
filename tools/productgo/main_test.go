package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Build a tiny real process with the product runtime. These controls exercise
// Notify/Stop and synchronous faults independently of Bashy's interpreter.
func TestProductRuntimeControls(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Unix runtime overlay")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	body := `package main
import("fmt";"os";"os/signal";"syscall")
func main(){
 if os.Args[1]=="fault" { var p *int; fmt.Println(*p); return }
 if os.Args[1]=="notify" {
  ch:=make(chan os.Signal,1); signal.Notify(ch,syscall.SIGTERM)
  if err:=syscall.Kill(os.Getpid(),syscall.SIGTERM);err!=nil{panic(err)}
  <-ch; signal.Stop(ch)
 }
 if err:=syscall.Kill(os.Getpid(),syscall.SIGTERM);err!=nil{panic(err)}
 fmt.Print("survived")
}`
	if err := os.WriteFile(source, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	for _, active := range []string{"1", "0"} {
		t.Run("activation="+active, func(t *testing.T) {
			bin := filepath.Join(dir, "probe"+active)
			build := exec.Command("../../scripts/go-product.sh", "build", "-o", bin, source)
			if active == "0" {
				cache, err := goCommand("env", "GOMODCACHE").Output()
				if err != nil {
					t.Fatal(err)
				}
				root, err := overlayRoot(runtime.GOROOT(), strings.TrimSpace(string(cache)), dir)
				if err != nil {
					t.Fatal(err)
				}
				// Compile the same overlay without any activation flag, outside
				// the product wrapper; the runtime must default to stock behavior.
				original, err := os.ReadFile(filepath.Join(root, "src/runtime/signal_unix.go"))
				if err != nil {
					t.Fatal(err)
				}
				patched, err := patchRuntimeSource(original)
				if err != nil {
					t.Fatal(err)
				}
				replacement := filepath.Join(dir, "signal_unix.go")
				if err := os.WriteFile(replacement, []byte(patched), 0600); err != nil {
					t.Fatal(err)
				}
				overlay, _ := json.Marshal(map[string]any{"Replace": map[string]string{filepath.Join(root, "src/runtime/signal_unix.go"): replacement}})
				path := filepath.Join(dir, "overlay.json")
				if err := os.WriteFile(path, overlay, 0600); err != nil {
					t.Fatal(err)
				}
				build = localGoCommand(root, "build", "-overlay="+path, "-o", bin, source)
			}
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("build: %v\n%s", err, out)
			}
			for _, mode := range []string{"ignore", "notify", "fault"} {
				if active == "0" && mode == "notify" {
					continue
				}
				t.Run(mode, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
					defer cancel()
					cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `ulimit -c 0; trap '' TERM; exec "$1" "$2"`, "sh", bin, mode)
					out, err := cmd.CombinedOutput()
					switch {
					case mode == "fault":
						if err == nil || !strings.Contains(string(out), "invalid memory address") {
							t.Fatalf("fault lost: err=%v out=%s", err, out)
						}
					case active == "0":
						if err == nil {
							t.Fatalf("inactive runtime unexpectedly preserved ignore: %s", out)
						}
					default:
						if err != nil || string(out) != "survived" {
							t.Fatalf("err=%v out=%s", err, out)
						}
					}
				})
			}
		})
	}
}

func TestProductBuildRejectsContractOverride(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Unix runtime overlay")
	}
	for _, args := range [][]string{{"build", "-ldflags=-X runtime.bashyInheritedIgnore=0", "."}, {"run", ".", "--", "-ldflags=program-argument"}, {"test", "."}} {
		out, err := exec.Command("../../scripts/go-product.sh", args...).CombinedOutput()
		if err == nil || (!strings.Contains(string(out), "cannot override") && !strings.Contains(string(out), "only product builds")) {
			t.Fatalf("args=%q err=%v out=%s", args, err, out)
		}
	}
	if _, err := patchRuntimeSource([]byte("unknown source")); err == nil {
		t.Fatal("accepted unknown runtime source")
	}
}

func TestManagedGoWithoutHostGoOnPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX managed front door")
	}
	dir := t.TempDir()
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	// The front door owns Go provisioning. PATH intentionally has no go (or cc).
	for _, name := range []string{"env", "dirname"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	front := filepath.Join(dir, "managed-go")
	script := "#!/bin/sh\n[ \"$1\" = go ] || exit 91\nshift\nexec '" + strings.ReplaceAll(realGo, "'", "'\\''") + "' \"$@\"\n"
	if err := os.WriteFile(front, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "main.go")
	if err := os.WriteFile(source, []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("../../scripts/go-product.sh", "build", "-o", filepath.Join(dir, "probe"), source)
	cmd.Env = append(os.Environ(), "PATH="+dir, "BASHY_EXE="+front, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("managed-only build: %v\n%s", err, out)
	}
}

func TestProductTrimpathContract(t *testing.T) {
	for _, value := range []string{"false", "0", "F", "invalid"} {
		t.Run(value, func(t *testing.T) {
			for _, inEnv := range []bool{false, true} {
				args := []string{"build", "."}
				env := append(os.Environ(), "GOFLAGS=")
				if inEnv {
					env = append(env, "GOFLAGS=-trimpath="+value)
				} else {
					args = []string{"build", "-trimpath=" + value, "."}
				}
				cmd := exec.Command("../../scripts/go-product.sh", args...)
				cmd.Env = env
				out, err := cmd.CombinedOutput()
				// Invalid GOFLAGS can be rejected by the bootstrap Go command itself.
				if err == nil || (!strings.Contains(string(out), "require -trimpath") && !(inEnv && value == "invalid" && strings.Contains(string(out), "invalid"))) {
					t.Fatalf("env=%v: %v %s", inEnv, err, out)
				}
			}
		})
	}
	for _, flag := range []string{`"-trimpath=false"`, "'-trimpath=0'"} {
		if err := requireTrimpath([]string{flag}); err == nil {
			t.Fatalf("accepted %s", flag)
		}
	}
	for _, flag := range []string{"-trimpath", "-trimpath=true", "-trimpath=1", "-trimpath=T"} {
		if err := requireTrimpath([]string{flag}); err != nil {
			t.Fatal(err)
		}
	}
}
