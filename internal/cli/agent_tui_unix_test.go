//go:build unix

package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty/v2"
)

// TestAgentTUILadderOverPTY is the T4 acceptance (Sprint #301): `bashy ycode`
// on a terminal hosts the agent in bashy's interactive shell, and every line
// goes through the ladder — a literal command runs as typed, a typo is
// repaired and echoed, free text becomes a turn (it reaches the model), a
// session command moves the session, and a leading "/" is no command
// language: it is a path like in any shell.
func TestAgentTUILadderOverPTY(t *testing.T) {
	bin := builtBashyBin(t)
	config, err := filepath.Abs("../../../ycode/examples/agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config); err != nil {
		t.Skipf("the ycode sibling is not mounted: %v", err)
	}

	// The model endpoint records what reaches it and answers every request
	// with one fixed text (the Responses stream), so a turn shows up as a
	// request carrying the typed text and an answer on the terminal.
	var mu sync.Mutex
	var requests []string
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"STUB-ANSWER\"}\n\n")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
	}))
	defer model.Close()
	reached := func(text string) bool {
		mu.Lock()
		defer mu.Unlock()
		for _, body := range requests {
			if strings.Contains(body, text) {
				return true
			}
		}
		return false
	}

	home := t.TempDir()
	cmd := exec.Command(bin, "ycode", "-f", config)
	cmd.Dir = t.TempDir()
	cmd.Env = []string{
		"HOME=" + home, "BASHY_HOME=" + filepath.Join(home, ".bashy"),
		"PATH=" + filepath.Dir(bin) + ":/bin:/usr/bin", "PS1=T> ", "TERM=xterm",
		"OPENAI_BASE_URL=" + model.URL, "OPENAI_API_KEY=stub", "BASHY_HINTS=off",
	}
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	capture := startPTYCapture(ptmx)
	t.Cleanup(func() {
		_ = ptmx.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	const wait = 20 * time.Second
	prompt := []byte("T> ")
	first := capture.waitFor(t, prompt, wait)
	session := regexp.MustCompile(`\[ycode ([0-9a-f]{8})\] T> `).FindSubmatch(first)
	if session == nil {
		t.Fatalf("the prompt does not carry the agent and session: %q", first)
	}
	if !bytes.Contains(first, []byte("ask in plain words")) {
		t.Fatalf("no greeting: %q", first)
	}
	send := func(line string) []byte {
		t.Helper()
		offset := capture.len()
		if _, err := io.WriteString(ptmx, line+"\r"); err != nil {
			t.Fatal(err)
		}
		// The echoed input line itself holds no prompt; the next one does.
		return capture.waitForFrom(t, offset+len(line), prompt, wait)
	}

	// Rung 0: a literal command runs as typed.
	if out := send("echo LIT-$((1+2))"); !bytes.Contains(out, []byte("LIT-3")) {
		t.Fatalf("literal command did not run: %q", out)
	}
	// Rung 1: a misspelled command name is repaired, echoed, then run.
	out := send("ehco REPAIRED")
	if !bytes.Contains(out, []byte("echo REPAIRED  (ehco → echo)")) || !bytes.Contains(out, []byte("\r\nREPAIRED\r\n")) {
		t.Fatalf("typo not repaired and echoed: %q", out)
	}
	// No slash commands: "/help" is a path, run by the shell, never a turn.
	send("/help")
	if reached("/help") {
		t.Fatal(`"/help" reached the model`)
	}
	// Last rung: free text becomes an agent turn.
	out = send("what does this repo do?")
	if !reached("what does this repo do?") {
		t.Fatalf("free text never reached the model; requests: %q; transcript: %q", requests, out)
	}
	if !bytes.Contains(out, []byte("STUB-ANSWER")) {
		t.Fatalf("the turn's answer is not on the terminal: %q", out)
	}
	// A session command moves the terminal's session.
	out = send("bashy ycode new")
	fresh := regexp.MustCompile(`([0-9a-f]{8})-[0-9a-f]{4}-`).FindSubmatch(out)
	if fresh == nil || bytes.Equal(fresh[1], session[1]) {
		t.Fatalf("`bashy ycode new` printed no new session: %q", out)
	}
	if !bytes.Contains(out, []byte("[ycode "+string(fresh[1])+"] T> ")) {
		t.Fatalf("the prompt did not move to the new session %s: %q", fresh[1], out)
	}
	if out := send("bashy ycode status"); !bytes.Contains(out, fresh[1]) {
		t.Fatalf("status does not report the terminal's session: %q", out)
	}

	if os.Getenv("AGENT_TUI_TRANSCRIPT") != "" {
		capture.mu.Lock()
		t.Logf("transcript:\n%s", capture.buf.String())
		capture.mu.Unlock()
	}
	if _, err := io.WriteString(ptmx, "exit\r"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("bashy ycode exit: %v", err)
		}
	case <-time.After(wait):
		t.Fatal("the agent terminal did not exit")
	}
	if left, _ := filepath.Glob(filepath.Join(home, ".bashy", "ycode", "terminals", "*")); len(left) != 0 {
		t.Fatalf("session pointer left behind: %v", left)
	}
}
