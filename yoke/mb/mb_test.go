package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/fleet/fleettest"
	"github.com/qiangli/yoke/pkg/principal"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/qiangli/yoke/pkg/weave"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	wireMessageBoard()
	if bus.IsMainLineTool == nil || bus.CurrentSessionClaim == nil {
		t.Fatal("wireMessageBoard must connect IsMainLineTool and CurrentSessionClaim")
	}
	cmd := bus.NewMessageBoardCmdWithSprintResolver(weave.ResolveSprintScopeByHandle)
	wrapAuthoredClaimErrors(cmd)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy mb:", nil)
	if code != 0 {
		t.Fatalf("mb --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("mb --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "mb")
}

func TestMBExtensionAndMainLineWiring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BASHY_HOME", filepath.Join(home, ".bashy"))
	t.Setenv("BASHY_FLEET_DIR", filepath.Join(home, ".config", "bashy"))
	t.Setenv("BASHY_BOARD_DIR", filepath.Join(home, "board"))
	t.Setenv("BASHY_ROOM_DIR", filepath.Join(home, "room"))
	t.Setenv("BASHY_INSTANCE_DIR", filepath.Join(home, "instances"))
	t.Setenv("BASHY_PRINCIPAL", "")
	t.Setenv("BASHY_INSTANCE", "")
	t.Setenv("BASHY_INSTANCE_UUID", "")
	t.Setenv("BASHY_SESSION_CLAIM", "")
	fleettest.Ring(t)
	wireMessageBoard()

	if !bus.IsMainLineTool("claude") {
		t.Fatal("expected IsMainLineTool(claude) = true")
	}
	if bus.IsMainLineTool("definitely-not-a-tool-xyz") {
		t.Fatal("expected unknown tool IsMainLineTool = false")
	}

	store := fleet.NewInstanceStore("")
	i2, err := store.Open(fleet.Family{Name: "claude", Display: "claude", Policy: fleet.PolicySingle, Bindings: []string{"claude:sonnet4.6"}}, fleet.OpenOptions{Label: "claude+2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := room.Join(room.Card{
		ID:        i2.ClaimID(),
		Nick:      "claude+2",
		Tool:      "claude",
		Binding:   "claude:sonnet4.6",
		Mode:      "interactive",
		PID:       os.Getpid(),
		OwnerPID:  os.Getpid(),
		Principal: principal.InstanceURN(i2.UUID),
	}); err != nil {
		t.Fatal(err)
	}

	cmd := bus.NewMessageBoardCmdWithSprintResolver(weave.ResolveSprintScopeByHandle)
	wrapAuthoredClaimErrors(cmd)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"send", "--as", "claude+2", "claude", "hello hunt group"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("mb send --as claude+2 claude: %v", err)
	}
	posts, err := bus.Posts()
	if err != nil || len(posts) != 1 {
		t.Fatalf("posts = %+v, err=%v", posts, err)
	}
	if posts[0].From != "claude+2" || !posts[0].IsHuntGroup() || posts[0].To != "claude" {
		t.Fatalf("unexpected post: %+v", posts[0])
	}
	if !strings.Contains(errBuf.String(), "claude") {
		t.Fatalf("unexpected send stderr: %s (stdout: %s)", errBuf.String(), out.String())
	}
}
