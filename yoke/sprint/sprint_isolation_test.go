package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/foreman"
	"github.com/qiangli/yoke/pkg/llmbudget"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/qiangli/yoke/pkg/weave"
)

// mkShortTempHome allocates a symlink-resolved temporary HOME whose path is
// short enough for Unix domain control sockets (~104 bytes on darwin) even
// with nested `.bashy/foreman/sprint-<id>-manager/ctl.sock` paths.
func mkShortTempHome(t testing.TB) string {
	t.Helper()
	base := ""
	if info, err := os.Stat("/tmp"); err == nil && info.IsDir() {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "bspr-")
	if err != nil {
		dir, err = os.MkdirTemp("", "bspr-")
	}
	if err != nil {
		t.Fatal(err)
	}
	if resolved, rerr := filepath.EvalSymlinks(dir); rerr == nil && resolved != "" {
		dir = resolved
	}
	t.Cleanup(func() {
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, werr error) error {
			if werr == nil && info != nil {
				_ = os.Chmod(p, 0o700)
			}
			return nil
		})
		_ = os.RemoveAll(dir)
	})
	return dir
}

// isolateSprintForemanTest defaults a sprint/foreman test to a temporary HOME,
// an isolated coordination/message-board bus, isolated foreman and supervisor
// paths, stage announcements off, and a fail-closed detached foreman launcher.
func isolateSprintForemanTest(t testing.TB) string {
	t.Helper()
	home := mkShortTempHome(t)
	bashyHome := filepath.Join(home, ".bashy")
	fleetDir := filepath.Join(home, ".config", "bashy")

	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key == isolationSentinel || key == "BASHY_CHAT_INBOX_MAX" {
			continue
		}
		if strings.HasPrefix(key, "BASHY_") || strings.HasPrefix(key, "WEAVE_") {
			t.Setenv(key, "")
		}
	}

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("BASHY_HOME", bashyHome)
	t.Setenv("BASHY_FLEET_DIR", fleetDir)
	if err := os.MkdirAll(fleetDir, 0o700); err != nil {
		t.Fatal(err)
	}

	for _, store := range []string{
		"SPRINT", "FOREMAN", "MB", "ROOM", "MEET", "MAILBOX",
		"COORD", "RESOURCES", "HANDOFF", "TODO", "SERVICES",
		"COMMANDS", "SKILLS", "SHIM", "INSTANCES",
	} {
		p := filepath.Join(bashyHome, strings.ToLower(store))
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BASHY_"+store+"_DIR", p)
	}
	t.Setenv("BASHY_SUPERVISORD_SOCKET", filepath.Join(bashyHome, "supervisord.sock"))
	t.Setenv("BASHY_SPRINT_ANNOUNCE", "0")

	t.Cleanup(llmbudget.SetDefault(llmbudget.New(llmbudget.Config{
		StatePath: filepath.Join(bashyHome, "budget.json"),
	})))

	prevDeliver := deliverSessionMail
	deliverSessionMail = func(context.Context, string, string) (weave.DeliveryReport, error) {
		return weave.DeliveryReport{Skipped: "test"}, nil
	}
	t.Cleanup(func() { deliverSessionMail = prevDeliver })

	prevRunOwner := runSprintOwnerCommand
	runSprintOwnerCommand = func(_ context.Context, path string, args []string, _ string, _ []string) ([]byte, error) {
		return nil, fmt.Errorf("refusing unstubbed sprint-owner launch in test: %s %s", path, strings.Join(args, " "))
	}
	t.Cleanup(func() { runSprintOwnerCommand = prevRunOwner })

	return home
}

func canonicalPath(p string) string {
	cleaned := filepath.Clean(p)
	cur := cleaned
	var tail []string
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return cleaned
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

func withinTempHome(path, tempHome string) bool {
	rel, err := filepath.Rel(canonicalPath(tempHome), canonicalPath(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

type externalPathSnapshot struct {
	exists  bool
	entries map[string]string
}

func takeExternalSnapshot(root string) (externalPathSnapshot, error) {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return externalPathSnapshot{exists: false}, nil
	}
	if err != nil {
		return externalPathSnapshot{}, err
	}
	entries := map[string]string{}
	if !info.IsDir() {
		sig, serr := fileSignature(root, info)
		if serr != nil {
			return externalPathSnapshot{}, serr
		}
		entries["."] = sig
		return externalPathSnapshot{exists: true, entries: entries}, nil
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			if os.IsNotExist(werr) {
				return nil
			}
			return werr
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil || rel == "." {
			return rerr
		}
		fi, ierr := d.Info()
		if ierr != nil {
			if os.IsNotExist(ierr) {
				return nil
			}
			return ierr
		}
		sig, serr := fileSignature(p, fi)
		if serr != nil {
			return serr
		}
		entries[rel] = sig
		return nil
	})
	if err != nil {
		return externalPathSnapshot{}, err
	}
	return externalPathSnapshot{exists: true, entries: entries}, nil
}

func fileSignature(path string, info os.FileInfo) (string, error) {
	if info.Mode().IsRegular() {
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return "gone", nil
			}
			return "", err
		}
		sum := sha256.Sum256(b)
		return fmt.Sprintf("file:%d:%x", len(b), sum), nil
	}
	return fmt.Sprintf("mode:%s:%d", info.Mode().Type(), info.ModTime().UnixNano()), nil
}

type sprintPublishGuard struct {
	tempHome string
	watched  []string
	before   map[string]externalPathSnapshot
}

func newSprintPublishGuard(tempHome string, externalPaths ...string) (*sprintPublishGuard, error) {
	g := &sprintPublishGuard{
		tempHome: canonicalPath(tempHome),
		before:   make(map[string]externalPathSnapshot, len(externalPaths)),
	}
	seen := map[string]bool{}
	for _, raw := range externalPaths {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		p := canonicalPath(raw)
		if withinTempHome(p, g.tempHome) || seen[p] {
			continue
		}
		seen[p] = true
		snap, err := takeExternalSnapshot(p)
		if err != nil {
			return nil, err
		}
		g.watched = append(g.watched, p)
		g.before[p] = snap
	}
	return g, nil
}

func (g *sprintPublishGuard) Verify() error {
	var violations []string

	if home, err := os.UserHomeDir(); err != nil {
		violations = append(violations, fmt.Sprintf("os.UserHomeDir failed: %v", err))
	} else if !withinTempHome(home, g.tempHome) {
		violations = append(violations, fmt.Sprintf("HOME %q escaped temp HOME %q", home, g.tempHome))
	}
	if got := room.Dir(); !withinTempHome(got, g.tempHome) {
		violations = append(violations, fmt.Sprintf("room.Dir() %q escaped temp HOME %q", got, g.tempHome))
	}
	if got := foreman.NewStore("", "sprint-1-manager").Dir(); !withinTempHome(got, g.tempHome) {
		violations = append(violations, fmt.Sprintf("foreman store %q escaped temp HOME %q", got, g.tempHome))
	}

	for _, envName := range []string{
		"BASHY_HOME", "BASHY_MB_DIR", "BASHY_ROOM_DIR", "BASHY_MEET_DIR",
		"BASHY_MAILBOX_DIR", "BASHY_COORD_DIR", "BASHY_SPRINT_DIR",
		"BASHY_FOREMAN_DIR", "BASHY_RESOURCES_DIR", "BASHY_HANDOFF_DIR",
		"BASHY_TODO_DIR", "BASHY_SERVICES_DIR", "BASHY_FLEET_DIR",
		"BASHY_SUPERVISORD_SOCKET",
	} {
		val := strings.TrimSpace(os.Getenv(envName))
		if val == "" {
			continue
		}
		if !withinTempHome(val, g.tempHome) {
			violations = append(violations, fmt.Sprintf("%s=%q escaped temp HOME %q", envName, val, g.tempHome))
		}
	}

	for _, p := range g.watched {
		before := g.before[p]
		after, err := takeExternalSnapshot(p)
		if err != nil {
			violations = append(violations, fmt.Sprintf("inspect external path %q: %v", p, err))
			continue
		}
		if !before.exists && after.exists {
			for rel := range after.entries {
				violations = append(violations, fmt.Sprintf("published outside temp HOME at %q", filepath.Join(p, rel)))
			}
			if len(after.entries) == 0 {
				violations = append(violations, fmt.Sprintf("created external path %q outside temp HOME", p))
			}
			continue
		}
		if before.exists && !after.exists {
			violations = append(violations, fmt.Sprintf("removed external path %q outside temp HOME", p))
			continue
		}
		for rel, afterSig := range after.entries {
			if beforeSig, ok := before.entries[rel]; !ok {
				violations = append(violations, fmt.Sprintf("published new artifact outside temp HOME: %q", filepath.Join(p, rel)))
			} else if beforeSig != afterSig {
				violations = append(violations, fmt.Sprintf("mutated external artifact outside temp HOME: %q", filepath.Join(p, rel)))
			}
		}
		for rel := range before.entries {
			if _, ok := after.entries[rel]; !ok {
				violations = append(violations, fmt.Sprintf("deleted external artifact outside temp HOME: %q", filepath.Join(p, rel)))
			}
		}
	}

	if len(violations) > 0 {
		return errors.New(strings.Join(violations, "; "))
	}
	return nil
}

// TestSprintForemanIsolationGuardFailsOnExternalPublishAndProtectsHost reproduces
// the 2026-11-06 Sprint 576 leak shape (sprint add/directive/take/move/start/instruct
// plus foreman session observation) and proves both halves of the contract:
//  1. Sprint/foreman helpers override poisoned ambient stores to a temp HOME with
//     an isolated bus, announce off, and detached foreman launch fenced.
//  2. The guard fails immediately if a sprint or foreman test publishes outside
//     its temp HOME.
func TestSprintForemanIsolationGuardFailsOnExternalPublishAndProtectsHost(t *testing.T) {
	t.Run("helpers isolate sprint board foreman and supervisor under temp HOME", func(t *testing.T) {
		ambientHome := mkShortTempHome(t)
		ambientBashy := filepath.Join(ambientHome, ".bashy")
		for _, sub := range []string{"mb", "room", "sprint", "foreman"} {
			if err := os.MkdirAll(filepath.Join(ambientBashy, sub), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		// Poison ambient env to look like an unisolated host shell with announce on.
		t.Setenv("HOME", ambientHome)
		t.Setenv("USERPROFILE", ambientHome)
		t.Setenv("BASHY_HOME", ambientBashy)
		t.Setenv("BASHY_MB_DIR", filepath.Join(ambientBashy, "mb"))
		t.Setenv("BASHY_ROOM_DIR", filepath.Join(ambientBashy, "room"))
		t.Setenv("BASHY_SPRINT_DIR", filepath.Join(ambientBashy, "sprint"))
		t.Setenv("BASHY_FOREMAN_DIR", filepath.Join(ambientBashy, "foreman"))
		t.Setenv("BASHY_SUPERVISORD_SOCKET", filepath.Join(ambientBashy, "supervisord.sock"))
		t.Setenv("BASHY_SPRINT_ANNOUNCE", "1")

		protected := []string{ambientHome}
		if testRealHomeSet && testRealHome != "" {
			protected = append(protected,
				filepath.Join(testRealHome, ".bashy", "mb", "posts.jsonl"),
				filepath.Join(testRealHome, ".bashy", "sprint", "queue.json"),
				filepath.Join(testRealHome, ".bashy", "foreman"),
			)
		}

		sprintWatchIsolate(t)
		tempHome := os.Getenv("HOME")
		if canonicalPath(tempHome) == canonicalPath(ambientHome) {
			t.Fatalf("helper kept poisoned ambient HOME %q", tempHome)
		}
		if got := os.Getenv("BASHY_SPRINT_ANNOUNCE"); got != "0" {
			t.Fatalf("BASHY_SPRINT_ANNOUNCE = %q, want 0", got)
		}

		guard, err := newSprintPublishGuard(tempHome, protected...)
		if err != nil {
			t.Fatal(err)
		}

		const owner = "test-agent"
		if err := fleet.New().SaveAgent(fleet.Agent{Name: owner, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
			t.Fatal(err)
		}
		wireMessageBoard()
		wireSprintOwner()

		execSprint := func(args ...string) (string, error) {
			cmd := newSprintCmd()
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs(args)
			err := cmd.Execute()
			return out.String() + errOut.String(), err
		}

		if out, err := execSprint("add", "Test sprint directive", "--primary-goal", "verify isolation", "--spec", "docs/test-plan.md"); err != nil {
			t.Fatalf("sprint add: %v\n%s", err, out)
		}
		if out, err := execSprint("directive", "add", "1", "never leak to real board", "--by", "operator"); err != nil {
			t.Fatalf("sprint directive add: %v\n%s", err, out)
		}
		if out, err := execSprint("take", "1", "--owner", owner); err != nil {
			t.Fatalf("sprint take: %v\n%s", err, out)
		}
		if out, err := execSprint("move", "1", "doing"); err != nil {
			t.Fatalf("sprint move: %v\n%s", err, out)
		}

		// An unstubbed managed-session launch must fail closed rather than
		// spawning a detached sprint-1-manager foreman daemon.
		if out, err := execSprint("start", "1", "--owner", owner, "--for", "1h", "--instruction", "Instruct test"); err == nil || !strings.Contains(out+err.Error(), "refusing unstubbed sprint-owner launch") {
			t.Fatalf("unstubbed sprint start --instruction = %v, output=%q; want fenced refusal", err, out)
		}

		// With stubbed owner seams, launch coordination locks must still stay inside tempHome.
		oldRun, oldLoad, oldTell, oldStop := runSprintOwnerForeman, loadSprintOwnerState, sendSprintOwnerTell, stopSprintOwnerForeman
		oldControl, oldTransport, oldSupported := waitSprintOwnerControl, waitSprintOwnerTransport, sprintOwnerControlSupported
		t.Cleanup(func() {
			runSprintOwnerForeman, loadSprintOwnerState = oldRun, oldLoad
			sendSprintOwnerTell, stopSprintOwnerForeman = oldTell, oldStop
			waitSprintOwnerControl, waitSprintOwnerTransport = oldControl, oldTransport
			sprintOwnerControlSupported = oldSupported
		})
		sprintOwnerControlSupported = func() bool { return true }
		loadSprintOwnerState = func(string) (foreman.State, error) { return foreman.State{}, os.ErrNotExist }
		runSprintOwnerForeman = func(context.Context, string, weave.SprintOwnerRequest) error { return nil }
		waitSprintOwnerControl = func(context.Context, string, time.Duration) error { return nil }
		sendSprintOwnerTell = func(string, string) (foreman.Ack, error) {
			return foreman.Ack{OK: true, Accepted: true}, nil
		}
		waitSprintOwnerTransport = func(context.Context, string, string, time.Duration) (room.OwnerTransport, error) {
			return room.TransportManaged, nil
		}
		stopSprintOwnerForeman = func(string) error { return nil }

		if _, err := startSprintOwner(context.Background(), weave.SprintOwnerRequest{
			Sprint: 1, Owner: owner, Brief: "E2E directive sprint",
		}); err != nil {
			t.Fatalf("stubbed startSprintOwner: %v", err)
		}
		if stop := foreman.ObserveSession(context.Background(), "sprint-1-manager", owner); stop != nil {
			stop()
		}

		// Stage announcements defaulted off: isolated board received zero posts.
		posts, err := bus.Posts()
		if err != nil {
			t.Fatal(err)
		}
		if len(posts) != 0 {
			t.Fatalf("isolated sprint operations emitted %d stage announcement(s) with announce default off: %+v", len(posts), posts)
		}

		lockPath := filepath.Join(os.Getenv("BASHY_FOREMAN_DIR"), "sprint-1-manager.launch.lock")
		if _, err := os.Stat(lockPath); err != nil {
			t.Fatalf("expected launch lock inside temp HOME at %s: %v", lockPath, err)
		}
		if err := guard.Verify(); err != nil {
			t.Fatalf("isolated sprint/foreman operations escaped temp HOME: %v", err)
		}
	})

	t.Run("guard fails if a sprint test publishes to board outside temp HOME", func(t *testing.T) {
		tempHome := isolateSprintForemanTest(t)
		escapedRoot := mkShortTempHome(t)
		guard, err := newSprintPublishGuard(tempHome, escapedRoot)
		if err != nil {
			t.Fatal(err)
		}

		// Simulate a misconfigured sprint test publishing stage announcements outside tempHome.
		escapedMB := filepath.Join(escapedRoot, "mb")
		if err := os.MkdirAll(escapedMB, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BASHY_MB_DIR", escapedMB)
		t.Setenv("BASHY_SPRINT_ANNOUNCE", "1")
		t.Setenv("BASHY_PRINCIPAL", "stage-author")
		fresh := time.Now().UTC()
		seedSprintBoard(t, []map[string]any{
			seededStory(1, "stage-author", fresh),
			seededStory(2, "stage-peer", fresh),
		})
		wireMessageBoard()
		runStageMove(t, "backlog")

		err = guard.Verify()
		if err == nil {
			t.Fatal("guard succeeded even though sprint stage announcement published outside temp HOME")
		}
		if !strings.Contains(err.Error(), "BASHY_MB_DIR") || !strings.Contains(err.Error(), "posts.jsonl") {
			t.Fatalf("guard failure must identify both escaped env and external board file, got: %v", err)
		}
	})

	t.Run("guard fails if foreman coordination writes outside temp HOME", func(t *testing.T) {
		isolateSprintOwnerHostSeams(t)
		tempHome := os.Getenv("HOME")
		escapedRoot := mkShortTempHome(t)
		guard, err := newSprintPublishGuard(tempHome, escapedRoot)
		if err != nil {
			t.Fatal(err)
		}

		escapedForeman := filepath.Join(escapedRoot, "foreman")
		if err := os.MkdirAll(escapedForeman, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BASHY_FOREMAN_DIR", escapedForeman)
		loadSprintOwnerState = func(string) (foreman.State, error) { return foreman.State{}, os.ErrNotExist }
		runSprintOwnerForeman = func(context.Context, string, weave.SprintOwnerRequest) error { return nil }
		waitSprintOwnerControl = func(context.Context, string, time.Duration) error { return nil }
		sendSprintOwnerTell = func(string, string) (foreman.Ack, error) {
			return foreman.Ack{OK: true, Accepted: true}, nil
		}
		waitSprintOwnerTransport = func(context.Context, string, string, time.Duration) (room.OwnerTransport, error) {
			return room.TransportManaged, nil
		}

		if _, err := startSprintOwner(context.Background(), weave.SprintOwnerRequest{
			Sprint: 42, Owner: "manager", Brief: "check",
		}); err != nil {
			t.Fatal(err)
		}

		err = guard.Verify()
		if err == nil {
			t.Fatal("guard succeeded even though foreman lockfile wrote outside temp HOME")
		}
		if !strings.Contains(err.Error(), "BASHY_FOREMAN_DIR") || !strings.Contains(err.Error(), "sprint-42-manager.launch.lock") {
			t.Fatalf("guard failure must identify escaped foreman lockfile, got: %v", err)
		}
	})
}
