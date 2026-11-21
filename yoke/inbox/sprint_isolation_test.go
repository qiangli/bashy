package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/llmbudget"
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
