package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise a real SDK under GOMODCACHE even on hosts using an installed SDK.
// This layout used to fail before compilation with Go's overlay prohibition.
func TestProductDownloadedToolchain(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Unix runtime overlay")
	}
	dir := t.TempDir()
	cache := filepath.Join(dir, "modcache")
	root := filepath.Join(cache, "golang.org", "toolchain@v0.0.1-go1.27.1."+runtime.GOOS+"-"+runtime.GOARCH)
	if err := os.CopyFS(root, os.DirFS(runtime.GOROOT())); err != nil {
		t.Fatal(err)
	}
	signal := filepath.Join(root, "src/runtime/signal_unix.go")
	before, err := os.ReadFile(signal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(signal, 0444); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(signal)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "main.go")
	if err := os.WriteFile(source, []byte(`package main
import("fmt";"os";"os/signal";"syscall")
func main(){
 ch:=make(chan os.Signal,1); signal.Notify(ch,syscall.SIGTERM)
 if err:=syscall.Kill(os.Getpid(),syscall.SIGTERM);err!=nil{panic(err)}
 <-ch; signal.Stop(ch)
 if err:=syscall.Kill(os.Getpid(),syscall.SIGTERM);err!=nil{panic(err)}
 fmt.Print("survived")
}`), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "probe")
	tmp := filepath.Join(dir, "private")
	if err := os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("../../scripts/go-product.sh", "build", "-o", bin, source)
	cmd.Env = append(os.Environ(), "BASHY_EXE=", "GOROOT="+root, "GOMODCACHE="+cache,
		"GOTOOLCHAIN=go1.27.1", "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+tmp, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cached SDK product build: %v\n%s", err, out)
	}
	run := exec.Command("/bin/sh", "-c", `trap '' TERM; exec "$1"`, "sh", bin)
	if out, err := run.CombinedOutput(); err != nil || string(out) != "survived" {
		t.Fatalf("runtime contract: %v\n%s", err, out)
	}
	after, err := os.ReadFile(signal)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(signal)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || info.Mode() != originalInfo.Mode() || !info.ModTime().Equal(originalInfo.ModTime()) {
		t.Fatal("shared runtime source was modified")
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "bashy-runtime-overlay-") {
			t.Fatal("private SDK leaked")
		}
	}
}

func TestOverlayRootIsolation(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	root := filepath.Join(cache, "sdk")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("stock"), 0500); err != nil {
		t.Fatal(err)
	}
	private, err := overlayRoot(root, cache, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "file"), []byte("private"), 0700); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(root, "file"))
	if err != nil || string(original) != "stock" {
		t.Fatalf("shared file modified: %q %v", original, err)
	}
	if _, err := overlayRoot(root, cache, cache); err == nil {
		t.Fatal("accepted private SDK inside cache")
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(cache, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := overlayRoot(filepath.Join(alias, "sdk"), alias, cache); err == nil {
		t.Fatal("accepted aliased cache destination")
	}
	sibling := filepath.Join(dir, "cache-sibling")
	if err := os.Mkdir(sibling, 0700); err != nil {
		t.Fatal(err)
	}
	got, err := overlayRoot(sibling, cache, dir)
	canonical, _ := filepath.EvalSymlinks(sibling)
	if err != nil || got != canonical {
		t.Fatalf("unnecessary copy: %q %v", got, err)
	}
}
