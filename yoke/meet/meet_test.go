package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/meet"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	wireMeet()
	cmd := meet.NewMeetCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy meet:", nil)
	if code != 0 {
		t.Fatalf("meet --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("meet --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "meet")
}

func TestWireMeetInstallsEverySeam(t *testing.T) {
	wireMeet()
	for name, set := range map[string]bool{
		"StartPermanentRole":    meet.StartPermanentRole != nil,
		"StartRoomSecretary":    meet.StartRoomSecretary != nil,
		"ValidateRoomSecretary": meet.ValidateRoomSecretary != nil,
		"Notify":                meet.Notify != nil,
		"FetchMB":               meet.FetchMB != nil,
		"PostMB":                meet.PostMB != nil,
	} {
		if !set {
			t.Errorf("meet.%s is nil after wireMeet — the seam exists but nothing connects it", name)
		}
	}
}

func TestMeetLazyStewardStartsRoomRoleWithoutTakingHostAuthority(t *testing.T) {
	oldSelect, oldEnsure, oldRoom := selectMeetAgent, ensureMeetPermanentRoleRoom, loadMeetRoom
	t.Cleanup(func() {
		selectMeetAgent, ensureMeetPermanentRoleRoom = oldSelect, oldEnsure
		loadMeetRoom = oldRoom
	})
	loadMeetRoom = func(string) (*meet.State, *meet.Synthesis, error) {
		return &meet.State{}, nil, nil
	}
	var gotName string
	selectMeetAgent = func(name, tool string, band int) (*stewardSelection, error) {
		gotName = name
		if tool != "" || band != 0 {
			t.Fatalf("named selector = %q %q %d", name, tool, band)
		}
		return &stewardSelection{Chosen: stewardCandidate{Name: "Rufus"}}, nil
	}
	var gotRoom, gotRole, gotHolder string
	ensureMeetPermanentRoleRoom = func(room, role, holder string, opts meet.CreateOptions) (*meet.State, error) {
		gotRoom, gotRole, gotHolder = room, role, holder
		if opts.Out != meet.OutStore {
			t.Fatalf("room output = %q", opts.Out)
		}
		return &meet.State{}, nil
	}
	if err := startMeetPermanentRole(context.Background(), meet.PermanentRoleStartRequest{
		Room: "steward", Role: "steward", Agent: "Rufus", Band: 4,
	}); err != nil {
		t.Fatal(err)
	}
	if gotName != "Rufus" || gotRoom != "steward" || gotRole != "steward" || gotHolder != "Rufus" {
		t.Fatalf("configured lazy assignment = name %q room %q role %q holder %q", gotName, gotRoom, gotRole, gotHolder)
	}

	selectMeetAgent = func(name, tool string, band int) (*stewardSelection, error) {
		if name != "" || tool != "" || band != 4 {
			t.Fatalf("band selector = %q %q %d", name, tool, band)
		}
		return &stewardSelection{
			Chosen:  stewardCandidate{Name: "secretary-agent"},
			Runners: []stewardCandidate{{Name: "Elif"}},
		}, nil
	}
	loadMeetRoom = func(string) (*meet.State, *meet.Synthesis, error) {
		return &meet.State{Secretary: "secretary-agent"}, nil, nil
	}
	oldRandom := stewardRandomIndex
	stewardRandomIndex = func(int) (int, error) { return 0, nil }
	t.Cleanup(func() { stewardRandomIndex = oldRandom })
	if err := startMeetPermanentRole(context.Background(), meet.PermanentRoleStartRequest{
		Room: "steward", Role: "steward", Band: 4,
	}); err != nil {
		t.Fatal(err)
	}
	if gotHolder != "Elif" {
		t.Fatalf("default lazy holder = %q", gotHolder)
	}
}

func TestMeetLazyStewardRejectsOnlySecretaryCandidate(t *testing.T) {
	oldSelect, oldRoom := selectMeetAgent, loadMeetRoom
	t.Cleanup(func() { selectMeetAgent, loadMeetRoom = oldSelect, oldRoom })
	selectMeetAgent = func(string, string, int) (*stewardSelection, error) {
		return &stewardSelection{Chosen: stewardCandidate{Name: "secretary-agent"}}, nil
	}
	loadMeetRoom = func(string) (*meet.State, *meet.Synthesis, error) {
		return &meet.State{Secretary: "secretary-agent"}, nil, nil
	}
	err := startMeetPermanentRole(context.Background(), meet.PermanentRoleStartRequest{
		Room: "steward", Role: "steward", Band: 4,
	})
	if err == nil || !strings.Contains(err.Error(), "excluding the secretary and chair") {
		t.Fatalf("conflicting steward selection = %v", err)
	}
}

func TestLazyStewardRandomizesOnlyAmongEligibleSelection(t *testing.T) {
	old := stewardRandomIndex
	stewardRandomIndex = func(n int) (int, error) { return n - 1, nil }
	t.Cleanup(func() { stewardRandomIndex = old })
	sel := &stewardSelection{
		Chosen:  stewardCandidate{Name: "a", Band: 4},
		Runners: []stewardCandidate{{Name: "b", Band: 4}, {Name: "c", Band: 4}},
	}
	if err := randomizeStewardSelection(sel); err != nil {
		t.Fatal(err)
	}
	if sel.Chosen.Name != "c" || len(sel.Runners) != 2 || !strings.Contains(sel.Why, "3 operable") {
		t.Fatalf("random selection = %+v", sel)
	}
}

func TestRoomSecretaryIsNamedFleetSelectionAndNotAnotherRoomRole(t *testing.T) {
	old := selectMeetAgent
	selectMeetAgent = func(name, tool string, band int) (*stewardSelection, error) {
		if name != "" || tool != "" || band != 2 {
			t.Fatalf("selector args = %q %q %d", name, tool, band)
		}
		return &stewardSelection{
			Chosen:  stewardCandidate{Name: "participant", Band: 4},
			Runners: []stewardCandidate{{Name: "secretary-agent", Band: 3}},
		}, nil
	}
	t.Cleanup(func() { selectMeetAgent = old })
	got, err := activateMeetRoomSecretary(context.Background(), meet.RoomSecretaryStartRequest{
		Room: "room-1", Band: 2, Exclude: []string{"participant"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "secretary-agent" {
		t.Fatalf("secretary = %q", got)
	}
}

func TestRoomSecretaryRejectsNamesOutsideBashyFleet(t *testing.T) {
	if err := validateMeetRoomSecretary("definitely-not-a-fleet-agent"); err == nil || !strings.Contains(err.Error(), "bashy agent list") {
		t.Fatalf("unknown secretary validation = %v", err)
	}
}
