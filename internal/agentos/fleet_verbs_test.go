// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// Sprint: #490; Story: #2607; Story-ID: 7bc375b8ad69

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// runFleetVerb executes one front-door verb tree in-process with captured
// output, mirroring dispatch (setCobraArgs + Execute) without exiting. The
// buffer is read after Execute returns, so help and refusal text land in it.
func runFleetVerb(t *testing.T, verb string, args ...string) (string, error) {
	t.Helper()
	cmd := cobraVerb(verb)
	if cmd == nil {
		t.Fatalf("cobraVerb(%q) is nil", verb)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	setCobraArgs(cmd, args)
	err := cmd.Execute()
	return out.String(), err
}

// TestTokenAndFleetVerbsResolve proves the S490.15b mount: `bashy token
// create --help` and `bashy fleet master-token mint --help` resolve through
// the same constructors dispatch runs, and both verbs are catalogued (bare
// shim), described (synopsis/schema), and classified (atlas effects for the
// MCP descriptors).
func TestTokenAndFleetVerbsResolve(t *testing.T) {
	ringDir(t)
	for _, verb := range []string{"token", "fleet"} {
		if !slices.Contains(alwaysShimVerbs, verb) {
			t.Errorf("%q not in alwaysShimVerbs (no bare shim, no catalog entry)", verb)
		}
		if synopsisOf(verb) == "" {
			t.Errorf("%q has no synopsis", verb)
		}
	}
	if _, _, verbs := commandsCatalog(); !slices.Contains(verbs, "token") || !slices.Contains(verbs, "fleet") {
		t.Errorf("token/fleet missing from the command catalog: %v", verbs)
	}

	out, err := runFleetVerb(t, "token", "create", "--help")
	if err != nil {
		t.Fatalf("token create --help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--to-secret") || !strings.Contains(out, "--stdout") {
		t.Errorf("token create --help hides the value sinks:\n%s", out)
	}

	out, err = runFleetVerb(t, "fleet", "master-token", "mint", "--help")
	if err != nil {
		t.Fatalf("fleet master-token mint --help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "never printed") {
		t.Errorf("fleet master-token mint --help hides the never-printed contract:\n%s", out)
	}

	for verb, want := range map[string][]string{
		"token": {"token", "token create", "token list", "token revoke"},
		"fleet": {"fleet", "fleet master-token", "fleet master-token save", "fleet master-token mint"},
	} {
		schemas := cobraSchemas(verb)
		for _, path := range want {
			if _, ok := schemas[path]; !ok {
				t.Errorf("%s schema missing %q", verb, path)
			}
		}
	}

	// Effects metadata the MCP descriptors carry: create = net+write, list
	// = net+read, revoke = net+write (union on the verb); master-token
	// mint = net+write+persist (union with save's read on the verb).
	tokenEffects := verbAtlasRecord("token", false).Effects
	for _, want := range []string{"net", "read", "write"} {
		if !slices.Contains(tokenEffects, want) {
			t.Errorf("token effects = %v, want %q", tokenEffects, want)
		}
	}
	fleetEffects := verbAtlasRecord("fleet", false).Effects
	for _, want := range []string{"net", "read", "write", "persist"} {
		if !slices.Contains(fleetEffects, want) {
			t.Errorf("fleet effects = %v, want %q", fleetEffects, want)
		}
	}
}

// TestSatelliteReproducePairedHostVerbsResolve proves the S490 #2644 mount
// (Story-ID 0a55048e5c6a): `bashy satellite invite --help`, `bashy
// reproduce --help`, and `bashy paired-host ls --help` resolve through the
// same constructors dispatch runs, and all three verbs are catalogued (bare
// shim), described (synopsis/schema), and classified (atlas effects for the
// MCP descriptors). paired-host is the front-door spelling of yoke's `host`
// tree: a bare `host` shim would shadow the platform DNS lookup.
func TestSatelliteReproducePairedHostVerbsResolve(t *testing.T) {
	ringDir(t)
	for _, verb := range []string{"satellite", "reproduce", "paired-host"} {
		if !slices.Contains(alwaysShimVerbs, verb) {
			t.Errorf("%q not in alwaysShimVerbs (no bare shim, no catalog entry)", verb)
		}
		if synopsisOf(verb) == "" {
			t.Errorf("%q has no synopsis", verb)
		}
	}
	if _, _, verbs := commandsCatalog(); !slices.Contains(verbs, "satellite") ||
		!slices.Contains(verbs, "reproduce") || !slices.Contains(verbs, "paired-host") {
		t.Errorf("satellite/reproduce/paired-host missing from the command catalog: %v", verbs)
	}

	out, err := runFleetVerb(t, "satellite", "invite", "--help")
	if err != nil {
		t.Fatalf("satellite invite --help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--satellite") || !strings.Contains(out, "--name") {
		t.Errorf("satellite invite --help hides the required flags:\n%s", out)
	}

	out, err = runFleetVerb(t, "reproduce", "--help")
	if err != nil {
		t.Fatalf("reproduce --help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--plan") {
		t.Errorf("reproduce --help hides the plan-only mode:\n%s", out)
	}

	out, err = runFleetVerb(t, "paired-host", "ls", "--help")
	if err != nil {
		t.Fatalf("paired-host ls --help: %v\n%s", err, out)
	}

	for verb, want := range map[string][]string{
		"satellite":   {"satellite", "satellite invite"},
		"reproduce":   {"reproduce"},
		"paired-host": {"paired-host", "paired-host ls"},
	} {
		schemas := cobraSchemas(verb)
		for _, path := range want {
			if _, ok := schemas[path]; !ok {
				t.Errorf("%s schema missing %q", verb, path)
			}
		}
	}

	// Effects metadata the MCP descriptors carry: satellite invite =
	// net+write (+read of the paired credential, +persist of the 0600
	// invite file); reproduce --plan and paired-host ls are net+read.
	satelliteEffects := verbAtlasRecord("satellite", false).Effects
	for _, want := range []string{"net", "read", "write", "persist"} {
		if !slices.Contains(satelliteEffects, want) {
			t.Errorf("satellite effects = %v, want %q", satelliteEffects, want)
		}
	}
	for _, verb := range []string{"reproduce", "paired-host"} {
		effects := verbAtlasRecord(verb, false).Effects
		for _, want := range []string{"net", "read"} {
			if !slices.Contains(effects, want) {
				t.Errorf("%s effects = %v, want %q", verb, effects, want)
			}
		}
	}
}

// TestTokenCreateRefusesWithoutAnOutputSink proves the minted value is never
// printed by default: without --to-secret or --stdout, create refuses before
// any network or mint, so no value can leak to the terminal. Hermetic: the
// refusal precedes client construction.
func TestTokenCreateRefusesWithoutAnOutputSink(t *testing.T) {
	ringDir(t)
	cmd := cobraVerb("token")
	if cmd == nil {
		t.Fatal("cobraVerb(token) is nil")
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	setCobraArgs(cmd, []string{"create", "--name", "x", "--scope", "tasks:read", "--ttl", "24h"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("token create without --to-secret/--stdout succeeded, output:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "--to-secret") || !strings.Contains(err.Error(), "--stdout") {
		t.Errorf("refusal names no safe sink: %v", err)
	}
	if got := out.String(); got != "" {
		t.Errorf("refusal printed %q (the value must never print)", got)
	}
}
