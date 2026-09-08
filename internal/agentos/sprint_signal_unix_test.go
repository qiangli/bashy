//go:build !windows

package agentos

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/room"
)

const sprintSignalOwner = "signal-watch-manager"

func TestSprintWatchSignalChild(t *testing.T) {
	if os.Getenv("SPRINT_SIGNAL_CHILD") != "1" {
		return
	}
	cmd := newSprintCmd()
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.SetArgs([]string{"start", "1", "--owner", sprintSignalOwner, "--watch", "--for", "10m", "--announce=false"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// A cancelled Go context alone cannot prove that the real CLI handles a signal:
// default SIGINT/SIGTERM termination skips every deferred cleanup callback.
func TestSprintWatchSignalsReleaseLeaseAndPreserveUnread(t *testing.T) {
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			sprintWatchIsolate(t)
			t.Setenv("WEAVE_CONDUCTOR", "")
			if err := fleet.New().SaveAgent(fleet.Agent{Name: sprintSignalOwner, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
				t.Fatal(err)
			}
			add := newSprintCmd()
			add.SetOut(&bytes.Buffer{})
			add.SetErr(&bytes.Buffer{})
			add.SetArgs([]string{"add", "signal cleanup proof", "--announce=false"})
			if err := add.Execute(); err != nil {
				t.Fatal(err)
			}
			if err := room.Emit(room.Event{Type: room.EventNotify, Actor: "fixture", Principal: "fixture", To: sprintSignalOwner, Body: "signal-unread-proof"}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSprintWatchSignalChild$")
			child.Env = append(os.Environ(), "SPRINT_SIGNAL_CHILD=1")
			var stderr bytes.Buffer
			child.Stderr = &stderr
			stdout, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
			ready := false
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				if strings.Contains(scanner.Text(), "signal-unread-proof") {
					ready = true
					break
				}
			}
			if !ready {
				t.Fatal("watcher never rendered pending input")
			}
			if err := child.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			if err := child.Wait(); err != nil {
				t.Fatalf("signal bypassed graceful cleanup: %v; %s", err, stderr.String())
			}
			show := newSprintCmd()
			var out bytes.Buffer
			show.SetOut(&out)
			show.SetErr(&out)
			show.SetArgs([]string{"show", "1", "--json"})
			if err := show.Execute(); err != nil {
				t.Fatal(err)
			}
			var result struct {
				Result struct {
					Sprint struct {
						Lease struct {
							Holder      string    `json:"holder"`
							At          time.Time `json:"at"`
							AttachedPID int       `json:"attached_pid"`
						} `json:"lease"`
					} `json:"sprint"`
				} `json:"result"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			lease := result.Result.Sprint.Lease
			if lease.Holder != sprintSignalOwner || !lease.At.IsZero() || lease.AttachedPID != 0 {
				t.Fatalf("signal cleanup must retract liveness and preserve holder: %+v", lease)
			}
			if _, err := os.Stat(filepath.Join(room.Dir(), "cursors", sprintSignalOwner)); !os.IsNotExist(err) {
				t.Fatalf("signal consumed unread mail or cursor inspection failed: %v", err)
			}
		})
	}
}
