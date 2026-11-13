// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// Sprint: #490; Story: #2607; Story-ID: 7bc375b8ad69

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
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

	out, err = runFleetVerb(t, "satellite", "list", "--help")
	if err != nil {
		t.Fatalf("satellite list --help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--json") {
		t.Errorf("satellite list --help hides --json:\n%s", out)
	}

	out, err = runFleetVerb(t, "satellite", "show", "--help")
	if err != nil {
		t.Fatalf("satellite show --help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--json") {
		t.Errorf("satellite show --help hides --json:\n%s", out)
	}

	out, err = runFleetVerb(t, "satellite", "transfer", "--help")
	if err != nil {
		t.Fatalf("satellite transfer --help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--to") || !strings.Contains(out, "--yes") {
		t.Errorf("satellite transfer --help hides --to/--yes:\n%s", out)
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
		"satellite":   {"satellite", "satellite list", "satellite show", "satellite invite", "satellite transfer"},
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

func TestMemberFrontDoorRegistration(t *testing.T) {
	ringDir(t)
	if !slices.Contains(alwaysShimVerbs, "member") {
		t.Fatal("member has no bare shim")
	}
	if synopsisOf("member") == "" {
		t.Fatal("member has no synopsis")
	}
	if _, _, verbs := commandsCatalog(); !slices.Contains(verbs, "member") {
		t.Fatal("member missing from command catalog")
	}
	for _, path := range []string{"member", "member ls", "member invite", "member accept", "member set", "member rm", "member of", "member invitations", "member check"} {
		if _, ok := cobraSchemas("member")[path]; !ok {
			t.Errorf("member schema missing %q", path)
		}
	}
	for _, tc := range []struct{ sub, flag string }{{"ls", "--json"}, {"invite", "--dry-run"}, {"accept", "--dry-run"}, {"set", "--dry-run"}, {"rm", "--dry-run"}, {"check", "--json"}} {
		out, err := runFleetVerb(t, "member", tc.sub, "--help")
		if err != nil || !strings.Contains(out, tc.flag) {
			t.Errorf("member %s --help missing %s: %v\n%s", tc.sub, tc.flag, err, out)
		}
	}
	if _, ok := cobraSchemas("member")["member completion"]; ok {
		t.Error("member unexpectedly exposes cobra completion")
	}
	for _, effect := range []string{"net", "read", "write"} {
		if !slices.Contains(verbAtlasRecord("member", false).Effects, effect) {
			t.Errorf("member atlas missing %s", effect)
		}
	}
}

func TestGroupFrontDoorRegistration(t *testing.T) {
	ringDir(t)
	if !slices.Contains(alwaysShimVerbs, "group") {
		t.Fatal("group has no bare shim")
	}
	if synopsisOf("group") == "" {
		t.Fatal("group has no synopsis")
	}
	if _, _, verbs := commandsCatalog(); !slices.Contains(verbs, "group") {
		t.Fatal("group missing from command catalog")
	}
	for _, path := range []string{"group", "group ls", "group add", "group rm", "group delete"} {
		if _, ok := cobraSchemas("group")[path]; !ok {
			t.Errorf("group schema missing %q", path)
		}
	}
	for _, tc := range []struct{ sub, flag string }{{"ls", "--json"}, {"add", "--dry-run"}, {"rm", "--dry-run"}, {"delete", "--dry-run"}} {
		out, err := runFleetVerb(t, "group", tc.sub, "--help")
		if err != nil || !strings.Contains(out, tc.flag) {
			t.Errorf("group %s --help missing %s: %v\n%s", tc.sub, tc.flag, err, out)
		}
	}
	if _, ok := cobraSchemas("group")["group completion"]; ok {
		t.Error("group unexpectedly exposes cobra completion")
	}
	for _, effect := range []string{"net", "read", "write"} {
		if !slices.Contains(verbAtlasRecord("group", false).Effects, effect) {
			t.Errorf("group atlas missing %s", effect)
		}
	}
}

func TestShareWhoFrontDoorRegistration(t *testing.T) {
	ringDir(t)
	schemas := cobraSchemas("share")
	for _, path := range []string{"share", "share qr", "share who", "share add", "share ls", "share rm"} {
		if _, ok := schemas[path]; !ok {
			t.Errorf("share schema missing %q", path)
		}
	}
	out, err := runFleetVerb(t, "share", "who", "--help")
	if err != nil || !strings.Contains(out, "--json") {
		t.Errorf("share who --help missing --json: %v\n%s", err, out)
	}
	addOut, err := runFleetVerb(t, "share", "add", "--help")
	for _, flag := range []string{"--circle", "--group", "--email", "--dry-run"} {
		if err != nil || !strings.Contains(addOut, flag) {
			t.Errorf("share add --help missing %s: %v\n%s", flag, err, addOut)
		}
	}
	qrOut, err := runFleetVerb(t, "share", "qr", "--help")
	if err != nil || !strings.Contains(qrOut, "--png") {
		t.Errorf("share qr --help missing --png: %v\n%s", err, qrOut)
	}
}

func TestSuperadminFrontDoorRegistration(t *testing.T) {
	ringDir(t)
	schemas := cobraSchemas("superadmin")
	for _, path := range []string{"superadmin", "superadmin hosts", "superadmin hosts ls", "superadmin hosts rm"} {
		if _, ok := schemas[path]; !ok {
			t.Errorf("superadmin schema missing %q", path)
		}
	}
	if synopsisOf("superadmin") == "" {
		t.Error("superadmin has no synopsis")
	}
}

func TestSuiteFrontDoorRegistration(t *testing.T) {
	ringDir(t)
	if !slices.Contains(alwaysShimVerbs, "suite") || synopsisOf("suite") == "" {
		t.Fatal("suite is missing its bare shim or synopsis")
	}
	if _, _, verbs := commandsCatalog(); !slices.Contains(verbs, "suite") {
		t.Fatal("suite missing from command catalog")
	}
	for _, path := range []string{"suite", "suite ls", "suite show", "suite set", "suite add", "suite remove", "suite rm", "suite share", "suite unshare", "suite shares"} {
		if _, ok := cobraSchemas("suite")[path]; !ok {
			t.Errorf("suite schema missing %q", path)
		}
	}
	out, err := runFleetVerb(t, "suite", "share", "work", "--circle", "team", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("suite share dry run: %v", err)
	}
	var request struct {
		DryRun bool   `json:"dry_run"`
		Method string `json:"method"`
		Body   struct {
			Circles []string `json:"circles"`
			Groups  []string `json:"groups"`
			Emails  []string `json:"emails"`
		} `json:"body"`
	}
	if err := json.Unmarshal([]byte(out), &request); err != nil {
		t.Fatalf("suite share dry run JSON: %v: %s", err, out)
	}
	if !request.DryRun || request.Method != http.MethodPost || !slices.Equal(request.Body.Circles, []string{"team"}) {
		t.Errorf("suite share dry run = %+v", request)
	}
}

// TestTokenCreateRefusesWithoutAnOutputSink proves the minted value is never
// printed by default: without --to-secret or --stdout, create refuses before
// any network or mint, so no value can leak to the terminal. Hermetic: the
// refusal precedes client construction.
// TestPairedHostInviteAndLLMPeerSurface reconciles the S606 pair/attach
// surface on the bashy side: `paired-host invite` (the pair-once code for
// your OWN account), `llm peer add --email/--circle` (the member-bound peer
// credential), and `member invite --circle`. Help text names the flags, the
// cobra schemas carry the subcommand paths, and the atlas records carry the
// effects the MCP descriptors serve.
func TestPairedHostInviteAndLLMPeerSurface(t *testing.T) {
	ringDir(t)
	if syn := synopsisOf("paired-host"); !strings.Contains(syn, "invite") {
		t.Errorf("paired-host synopsis hides invite: %q", syn)
	}
	if syn := synopsisOf("llm"); !strings.Contains(syn, "peer") {
		t.Errorf("llm synopsis hides peer sharing: %q", syn)
	}

	out, err := runFleetVerb(t, "paired-host", "invite", "--help")
	if err != nil {
		t.Fatalf("paired-host invite --help: %v\n%s", err, out)
	}
	for _, want := range []string{"--dry-run", "--json", "own account"} {
		if !strings.Contains(out, want) {
			t.Errorf("paired-host invite --help hides %q:\n%s", want, out)
		}
	}

	out, err = runFleetVerb(t, "llm", "peer", "--help")
	if err != nil {
		t.Fatalf("llm peer --help: %v\n%s", err, out)
	}
	for _, want := range []string{"add", "share", "revoke", "limit"} {
		if !strings.Contains(out, want) {
			t.Errorf("llm peer --help hides %q:\n%s", want, out)
		}
	}

	out, err = runFleetVerb(t, "llm", "peer", "add", "--help")
	if err != nil {
		t.Fatalf("llm peer add --help: %v\n%s", err, out)
	}
	for _, want := range []string{"--email", "--circle", "--share", "--dry-run", "--json"} {
		if !strings.Contains(out, want) {
			t.Errorf("llm peer add --help hides %q:\n%s", want, out)
		}
	}

	out, err = runFleetVerb(t, "member", "invite", "--help")
	if err != nil {
		t.Fatalf("member invite --help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--circle") {
		t.Errorf("member invite --help hides --circle:\n%s", out)
	}

	for verb, want := range map[string][]string{
		"paired-host": {"paired-host", "paired-host ls", "paired-host invite"},
		"llm":         {"llm peer", "llm peer add", "llm peer set"},
		"member":      {"member invite", "member accept"},
	} {
		schemas := cobraSchemas(verb)
		for _, path := range want {
			if _, ok := schemas[path]; !ok {
				t.Errorf("%s schema missing %q", verb, path)
			}
		}
	}

	// paired-host invite POSTs a mint over the network: the union is
	// net+read+write.
	for _, want := range []string{"net", "read", "write"} {
		if !slices.Contains(verbAtlasRecord("paired-host", false).Effects, want) {
			t.Errorf("paired-host effects missing %q", want)
		}
	}
	// llm serves metered inference to peers: the shared atlas carries the
	// spend/net union the MCP descriptors serve.
	llmEffects := verbAtlasRecord("llm", false).Effects
	for _, want := range []string{"net", "spend"} {
		if !slices.Contains(llmEffects, want) {
			t.Errorf("llm effects = %v, want %q", llmEffects, want)
		}
	}
}

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

// TestComposedSatelliteOperationsWorkflow demonstrates the full composed workflow
// (S598 Story 44d88adca4a5) against synthetic fixtures:
// 1. Existing account discovery (discovering endpoints / accounts)
// 2. Satellite discovery and show (list --json, show <name> --json)
// 3. Paired-host inspection (paired-host ls --json; verify host-to-satellite gap refusal)
// 4. Explicit invite and transfer mutations:
//   - satellite invite --json includes pairing code even when written to file
//   - satellite transfer with explicit confirmation
//
// Verifies reads issue zero mutations, mutations occur only on explicit operations,
// and live service verification remains separate and unverified.
func TestComposedSatelliteOperationsWorkflow(t *testing.T) {
	ringDir(t)

	var (
		masterMutations atomic.Int32
		satMutations    atomic.Int32
	)

	var satSrv *httptest.Server
	satSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tokens/exchange":
			satMutations.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":      "synth-invite-token",
				"expires_at": "2026-11-09T00:00:00Z",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/hosts/invites":
			satMutations.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":       "inv-synth-pair-9876",
				"expires_at": "2026-11-08T19:00:00Z",
			})
		default:
			http.Error(w, "unexpected satellite path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(satSrv.Close)

	masterSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/satellites":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"satellites": []map[string]any{
					{
						"id":       "sat-synth-1",
						"name":     "sat-alpha",
						"endpoint": satSrv.URL,
						"state":    "active",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/satellites/sat-synth-1":
			// show reads the detail endpoint after resolving a name via the list.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"satellite": map[string]any{
					"id":       "sat-synth-1",
					"name":     "sat-alpha",
					"endpoint": satSrv.URL,
					"state":    "active",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/hosts":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"hosts": []map[string]any{
					{
						"host":   "host-synth-alpha",
						"online": true,
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/peers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"peers": []map[string]any{
					{
						"host":   "host-synth-alpha",
						"owned":  true,
						"online": true,
					},
				},
			})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/satellites/sat-synth-1/transfer"):
			masterMutations.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"satellite_id":   "sat-synth-1",
				"from":           "owner@example.internal",
				"to":             "new-owner@example.internal",
				"transferred_at": "2026-11-08T18:00:00Z",
				"note":           "transferred successfully",
			})
		default:
			http.Error(w, "unexpected master path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(masterSrv.Close)

	// Step 1: Account discovery
	acctOut, err := runFleetVerb(t, "account", "list", "--json")
	if err != nil {
		t.Fatalf("account list: %v\n%s", err, acctOut)
	}

	// Step 2: Satellite list and show
	listOut, err := runFleetVerb(t, "satellite", "list", "--json", "--master", masterSrv.URL, "--token", "fake-token")
	if err != nil {
		t.Fatalf("satellite list: %v\n%s", err, listOut)
	}
	var listEnvelope struct {
		Satellites []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Endpoint string `json:"endpoint"`
			State    string `json:"state"`
		} `json:"satellites"`
	}
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("unmarshal satellite list JSON: %v\n%s", err, listOut)
	}
	if len(listEnvelope.Satellites) != 1 || listEnvelope.Satellites[0].ID != "sat-synth-1" {
		t.Fatalf("unexpected satellites list: %+v", listEnvelope.Satellites)
	}

	showOut, err := runFleetVerb(t, "satellite", "show", "sat-alpha", "--json", "--master", masterSrv.URL, "--token", "fake-token")
	if err != nil {
		t.Fatalf("satellite show: %v\n%s", err, showOut)
	}
	var showEnvelope struct {
		Satellite struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Endpoint string `json:"endpoint"`
			State    string `json:"state"`
		} `json:"satellite"`
	}
	if err := json.Unmarshal([]byte(showOut), &showEnvelope); err != nil {
		t.Fatalf("unmarshal satellite show JSON: %v\n%s", err, showOut)
	}
	if showEnvelope.Satellite.ID != "sat-synth-1" || showEnvelope.Satellite.Name != "sat-alpha" {
		t.Fatalf("unexpected satellite show: %+v", showEnvelope.Satellite)
	}

	// Step 3: Paired-host inspection
	hostOut, err := runFleetVerb(t, "paired-host", "ls", "--json", "--master", masterSrv.URL, "--token", "fake-token")
	if err != nil {
		t.Fatalf("paired-host ls: %v\n%s", err, hostOut)
	}
	if !strings.Contains(hostOut, "host-synth-alpha") {
		t.Fatalf("paired-host ls output missing host-synth-alpha: %s", hostOut)
	}

	// Verify genuine architectural gap: --satellite filter is explicitly refused
	_, err = runFleetVerb(t, "paired-host", "ls", "--satellite", "sat-synth-1", "--master", masterSrv.URL, "--token", "fake-token")
	if err == nil || !strings.Contains(err.Error(), "GET /api/v1/satellites/assignment") {
		t.Fatalf("expected missing mapping refusal on paired-host ls --satellite, got: %v", err)
	}

	// Ensure zero mutations have been issued during discovery/reads
	if m := masterMutations.Load(); m != 0 {
		t.Fatalf("expected 0 master mutations during reads, got %d", m)
	}
	if s := satMutations.Load(); s != 0 {
		t.Fatalf("expected 0 satellite mutations during reads, got %d", s)
	}

	// Step 4: Explicit invite mutation
	inviteFile := filepath.Join(t.TempDir(), "host.invite")
	inviteOut, err := runFleetVerb(t, "satellite", "invite", "--satellite", satSrv.URL, "--token", "fake-token", "--name", "host-synth-alpha", "--to-file", inviteFile, "--json")
	if err != nil {
		t.Fatalf("satellite invite: %v\n%s", err, inviteOut)
	}
	// runFleetVerb merges stderr, which carries the owner-only note; the
	// JSON document is stdout.
	var inviteJSON map[string]any
	if err := json.Unmarshal([]byte(inviteOut[strings.Index(inviteOut, "{"):]), &inviteJSON); err != nil {
		t.Fatalf("unmarshal invite JSON: %v\n%s", err, inviteOut)
	}
	// Verify output compatibility: JSON output contains "code" even when --to-file is written
	if code, _ := inviteJSON["code"].(string); code != "inv-synth-pair-9876" {
		t.Fatalf("invite JSON missing pairing code: %v", inviteJSON)
	}
	if file, _ := inviteJSON["file"].(string); file != inviteFile {
		t.Fatalf("invite JSON missing target file: %v", inviteJSON)
	}
	fileBytes, err := os.ReadFile(inviteFile)
	if err != nil || strings.TrimSpace(string(fileBytes)) != "inv-synth-pair-9876" {
		t.Fatalf("written invite file mismatch: %s, err: %v", string(fileBytes), err)
	}

	// Explicit transfer mutation
	transferOut, err := runFleetVerb(t, "satellite", "transfer", "sat-alpha", "--to", "new-owner@example.internal", "--yes", "--master", masterSrv.URL, "--token", "fake-token")
	if err != nil {
		t.Fatalf("satellite transfer: %v\n%s", err, transferOut)
	}
	if !strings.Contains(transferOut, "satellite: sat-synth-1") || !strings.Contains(transferOut, "to: new-owner@example.internal") {
		t.Fatalf("unexpected transfer output: %s", transferOut)
	}

	// Assert mutations occurred exactly as planned
	if m := masterMutations.Load(); m != 1 {
		t.Fatalf("expected exactly 1 master transfer mutation, got %d", m)
	}
	if s := satMutations.Load(); s != 2 { // exchange + mint
		t.Fatalf("expected exactly 2 satellite mutations (exchange+mint), got %d", s)
	}
}
