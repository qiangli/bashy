//go:build unix

package agentos

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty/v2"
)

// Exercise the real no-config entry through the recipe and its journal. The
// dispatch-only test cannot detect a pipe substituted for the child's PTY.
func TestBareYcodeOpensGenieTUIOverPTY(t *testing.T) {
	root := t.TempDir()
	bar, err := filepath.Abs(filepath.Join("..", "..", "..", "ycode", "examples", "genie", "dist", "genie.bar"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bar); err != nil {
		if os.Getenv("GENIE_TUI_REQUIRED") == "1" {
			t.Fatalf("package the genie fixture bundle first: %v", err)
		}
		t.Skipf("package the genie fixture bundle first: %v", err)
	}
	bin := filepath.Join(root, "bashy")
	build := exec.Command("go", "build", "-o", bin, "./cmd/bashy")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build bashy: %v\n%s", err, out)
	}
	work := filepath.Join(root, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "ycode") // deliberately no -f
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"), "GENIE_BAR="+bar, "GENIE_EXTERNAL_MODEL=stub", "GENIE_MODEL_ID=stub-model", "GENIE_EXTERNAL_CONTEXT=32768", "OPENAI_BASE_URL=http://127.0.0.1:1/v1", "OPENAI_API_KEY=stub", "GENIE_TOOLCHAINS=none", "BASHY_HOME="+filepath.Join(root, "home"), "YCODE_CONFIG=", "TERM=xterm-256color")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()
	chunks := make(chan []byte, 32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunks <- bytes.Clone(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	var output bytes.Buffer
	for {
		select {
		case chunk := <-chunks:
			output.Write(chunk)
			if hasGenieTUIBanner(output.String()) {
				// A no-model turn is never submitted; Ctrl-D only leaves the TUI.
				_, _ = ptmx.Write([]byte{4})
				goto check
			}
		case <-done:
			goto check
		case <-deadline.C:
			cancel()
			_ = ptmx.Close()
			goto check
		}
	}
check:
	waitErr := cmd.Wait()
	got := output.String()
	if ctx.Err() != nil || waitErr != nil {
		t.Fatalf("bare ycode TUI did not exit cleanly: %v (context %v), output: %q", waitErr, ctx.Err(), got)
	}
	if !hasGenieTUIBanner(got) {
		t.Fatalf("bare ycode did not draw genie TUI on PTY: %q", got)
	}
}

func hasGenieTUIBanner(output string) bool {
	return strings.Contains(output, "genie · session ") &&
		strings.Contains(output, "ask in plain words") &&
		strings.Contains(output, "Ctrl-D leaves.")
}
