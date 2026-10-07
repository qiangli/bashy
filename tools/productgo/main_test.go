package main

import (
	"context"
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
			build := exec.Command("../../scripts/go-product.sh", "build", "-ldflags=-X runtime.bashyInheritedIgnore="+active, "-o", bin, source)
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
