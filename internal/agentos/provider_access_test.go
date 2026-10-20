// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
)

// TestProviderAccessCLIGolden pins the bashy half of S472.12: the `infra`
// verb is mounted exactly like `adapter` (dispatch, schema, help, atlas),
// and `infra add` registers provider access only — it never provisions
// anything, never changes artifact sourcing, never touches satellite
// authorization.
//
// The test runs the cobra command in-process (never execs the bashy self
// binary) against temp HOME/BASHY_HOME with no network:
//   - list shows the embedded digitalocean entry with origin ring, form,
//     version, protocol version and capabilities;
//   - add of a fixture executable manifest into the local ring, then show
//     reports origin local;
//   - an invalid manifest and a protocol mismatch exit with distinct codes.
func TestProviderAccessCLIGolden(t *testing.T) {
	home := t.TempDir()
	bashyHome := filepath.Join(t.TempDir(), "bashy-home")
	fleetDir := filepath.Join(t.TempDir(), "fleet")
	t.Setenv("HOME", home)
	t.Setenv("BASHY_HOME", bashyHome)
	t.Setenv("BASHY_FLEET_DIR", fleetDir)
	t.Setenv("BASHY_INFRA_DIR", "")
	t.Setenv("BASHY_INFRA_PATH", "")
	t.Setenv("BASHY_FLEET_TOKEN", "")
	t.Setenv("BASHY_API_KEY", "")
	t.Setenv("BASHY_FLEET_SEEDS", "")
	t.Setenv("VSC_PROFILE", "")

	// The verb is mounted everywhere `adapter` is: dispatch shim list,
	// cobra schema, help surface, synopsis table and atlas.
	if !slices.Contains(alwaysShimVerbs, "infra") {
		t.Fatal("infra not in alwaysShimVerbs (no bare shim, no catalog entry)")
	}
	if cobraVerb("infra") == nil {
		t.Fatal(`cobraVerb("infra") is nil (bashy infra --help would fail)`)
	}
	if !isHelpVerb("infra") {
		t.Fatal("infra is not a help verb (bashy help infra would fail)")
	}
	if strings.TrimSpace(verbSynopsis["infra"]) == "" {
		t.Fatal("infra has no synopsis (bashy commands would flag it)")
	}
	schemas := cobraSchemas("infra")
	if len(schemas) < 2 || schemas["infra"].Flags == nil && schemas["infra"].Positionals == nil {
		t.Fatalf("infra has no useful schema surface: %v", schemas)
	}
	found := false
	for _, name := range []string{"infra list", "infra show", "infra add", "infra rm"} {
		if _, ok := schemas[name]; ok {
			found = true
		}
	}
	if !found {
		t.Fatalf("infra schema missing kind-core verbs: %v", schemas)
	}

	runInfra := func(args ...string) (string, error) {
		t.Helper()
		cmd := cobraVerb("infra")
		if cmd == nil {
			t.Fatal(`cobraVerb("infra") is nil`)
		}
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), err
	}

	// List shows the embedded digitalocean reference entry: origin ring,
	// form, version, protocol version and capabilities. No network involved:
	// the entry ships compiled into the binary.
	out, err := runInfra("list")
	if err != nil {
		t.Fatalf("infra list: %v\n%s", err, out)
	}
	for _, want := range []string{
		"digitalocean", "embedded", "declarative", "provider/v1", "validate", "create",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("infra list missing %q:\n%s", want, out)
		}
	}

	// Add registers a fixture executable manifest into the local ring; show
	// then reports origin local. Nothing is provisioned or executed: add and
	// show only parse, validate and store the document.
	fixture := "kind: infra\nname: fixture-exec\nform: executable\n" +
		"protocol_version: provider/v1\nversion: \"1\"\n" +
		"capabilities: [create]\nargv: [\"/bin/fixture-exec\"]\n"
	fixturePath := filepath.Join(t.TempDir(), "fixture-exec.yaml")
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runInfra("add", fixturePath); err != nil {
		t.Fatalf("infra add fixture: %v\n%s", err, out)
	}
	out, err = runInfra("show", "fixture-exec")
	if err != nil {
		t.Fatalf("infra show fixture-exec: %v\n%s", err, out)
	}
	for _, want := range []string{"name: fixture-exec", "origin: local", "form: executable"} {
		if !strings.Contains(out, want) {
			t.Errorf("infra show fixture-exec missing %q:\n%s", want, out)
		}
	}

	// An invalid manifest and a protocol mismatch fail with distinct codes.
	invalidPath := filepath.Join(t.TempDir(), "invalid.yaml")
	invalid := "kind: infra\nname: badform\nform: telepathy\n" +
		"protocol_version: provider/v1\ncapabilities: [create]\n"
	if err := os.WriteFile(invalidPath, []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}
	_, invalidErr := runInfra("add", invalidPath)
	if invalidErr == nil {
		t.Fatal("infra add invalid manifest succeeded, want invalid_manifest")
	}
	mismatchPath := filepath.Join(t.TempDir(), "mismatch.yaml")
	mismatch := "kind: infra\nname: badproto\nform: declarative\n" +
		"protocol_version: provider/v9\nversion: \"1\"\ncapabilities: [create]\n" +
		"declarative:\n  lifecycle:\n    create: {endpoint: /v2/things}\n"
	if err := os.WriteFile(mismatchPath, []byte(mismatch), 0o644); err != nil {
		t.Fatal(err)
	}
	_, mismatchErr := runInfra("add", mismatchPath)
	if mismatchErr == nil {
		t.Fatal("infra add protocol mismatch succeeded, want protocol_mismatch")
	}
	invalidCode := fleet.InfraExitCode(invalidErr)
	mismatchCode := fleet.InfraExitCode(mismatchErr)
	if invalidCode != fleet.ExitInfraInvalid {
		t.Errorf("invalid manifest exit = %d, want %d (%v)", invalidCode, fleet.ExitInfraInvalid, invalidErr)
	}
	if mismatchCode != fleet.ExitInfraProtocolMismatch {
		t.Errorf("protocol mismatch exit = %d, want %d (%v)", mismatchCode, fleet.ExitInfraProtocolMismatch, mismatchErr)
	}
	if invalidCode == mismatchCode {
		t.Errorf("exit codes must be distinct: invalid=%d mismatch=%d", invalidCode, mismatchCode)
	}
}
