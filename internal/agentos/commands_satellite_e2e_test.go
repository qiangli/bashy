//go:build e2e

package agentos

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/qiangli/yoke/pkg/accounts"
)

// TestE2ESatelliteAccountAndTransferContract exercises the installed CLI with
// isolated accounts and HTTP services. It deliberately never uses host accounts.
func TestE2ESatelliteAccountAndTransferContract(t *testing.T) {
	bin := bashyBinary(t)
	env := append(scratchStoreEnv(t),
		"S598_OWNER_A=owner-a-fixture-token", "S598_OWNER_B=owner-b-fixture-token",
		"S598_INVITE_OWNER=invite-owner-fixture-token",
		"BASHY_CLOUDBOX_URL=http://127.0.0.1:1", "BASHY_FLEET_TOKEN=ambient-must-not-be-used",
	)
	var wrongAuth, ownerAPosts, ownerBPosts, invitePosts atomic.Int32
	ownerServer := func(id, name, token string, posts *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Header.Get("Authorization") != "Bearer "+token {
				wrongAuth.Add(1)
				http.Error(w, "credential mismatch", http.StatusForbidden)
				return
			}
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v1/satellites":
				_ = json.NewEncoder(w).Encode(map[string]any{"satellites": []map[string]any{{"id": id, "name": name, "state": "active"}}})
			case r.Method == http.MethodPost && r.URL.Path == "/api/v1/satellites/"+id+"/transfer":
				posts.Add(1)
				var body struct {
					To string `json:"to"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.To != "next@example.invalid" {
					http.Error(w, "request body mismatch", http.StatusBadRequest)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"satellite_id": id, "from": "owner@example.invalid", "to": body.To, "transferred_at": "2026-11-08T18:00:00Z", "note": "transferred"})
			default:
				http.NotFound(w, r)
			}
		}))
	}
	a := ownerServer("sat-fixture-a", "alpha", "owner-a-fixture-token", &ownerAPosts)
	b := ownerServer("sat-fixture-b", "beta", "owner-b-fixture-token", &ownerBPosts)
	t.Cleanup(a.Close)
	t.Cleanup(b.Close)
	invite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/tokens/exchange":
			if r.Header.Get("Authorization") != "Bearer invite-owner-fixture-token" {
				wrongAuth.Add(1)
				http.Error(w, "credential mismatch", http.StatusForbidden)
				return
			}
			var body struct {
				Scopes []string `json:"scopes"`
				TTL    string   `json:"ttl"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Scopes) != 1 || body.Scopes[0] != "host:invite-mint" || body.TTL == "" {
				http.Error(w, "exchange body mismatch", http.StatusBadRequest)
				return
			}
			invitePosts.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "short-fixture-token"})
		case "/api/v1/hosts/invites":
			if r.Header.Get("Authorization") != "Bearer short-fixture-token" {
				wrongAuth.Add(1)
				http.Error(w, "credential mismatch", http.StatusForbidden)
				return
			}
			var body struct {
				Name string `json:"name"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name != "fixture-host" {
				http.Error(w, "invite body mismatch", http.StatusBadRequest)
				return
			}
			invitePosts.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "fixture-invite-code"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(invite.Close)

	run := func(args ...string) (string, string, int) {
		out, errout, code := runBashyStdEnv(bin, env, args...)
		for _, secret := range []string{"owner-a-fixture-token", "owner-b-fixture-token", "invite-owner-fixture-token", "ambient-must-not-be-used"} {
			if strings.Contains(out, secret) || strings.Contains(errout, secret) {
				t.Fatalf("credential leaked by CLI operation %q", args[0])
			}
		}
		return out, errout, code
	}
	store := accounts.Open(filepath.Join(strings.TrimPrefix(env[0], "BASHY_HOME="), "accounts", "accounts.json"))
	add := func(alias string, kind accounts.Category, endpoint, credential string) {
		t.Helper()
		err := store.Observe("cloudbox", []accounts.Observation{{Account: accounts.Account{
			Alias: alias, Category: kind, Provider: "cloudbox", ProviderAccountID: alias,
			Connections: []accounts.Connection{{ID: "primary", Endpoint: endpoint, CredentialRef: "env:" + credential}},
		}}})
		if err != nil {
			t.Fatalf("observe fixture account %s: %v", alias, err)
		}
	}
	add("fixture-a", accounts.Cloudbox, a.URL, "S598_OWNER_A")
	add("fixture-b", accounts.Cloudbox, b.URL, "S598_OWNER_B")
	add("fixture-invite", accounts.Satellite, invite.URL, "S598_INVITE_OWNER")
	out, errout, code := run("account", "list", "--json")
	if code != 0 || !strings.Contains(out, "fixture-a") || !strings.Contains(out, "fixture-b") || !strings.Contains(out, "fixture-invite") {
		t.Fatalf("account discovery: exit %d: %s %s", code, out, errout)
	}

	for _, tc := range []struct{ alias, want, reject string }{{"fixture-a", "sat-fixture-a", "sat-fixture-b"}, {"fixture-b", "sat-fixture-b", "sat-fixture-a"}} {
		out, errout, code := run("satellite", "list", "--account", tc.alias, "--json")
		if code != 0 || !strings.Contains(out, tc.want) || strings.Contains(out, tc.reject) {
			t.Fatalf("isolated list %s: exit %d: %s %s", tc.alias, code, out, errout)
		}
	}
	out, errout, code = run("satellite", "show", "alpha", "--account", "fixture-a", "--json")
	if code != 0 || !strings.Contains(out, "sat-fixture-a") {
		t.Fatalf("account show: exit %d: %s %s", code, out, errout)
	}
	before := ownerAPosts.Load()
	out, errout, code = run("satellite", "list", "--account", "fixture-a", "--master", b.URL)
	if code == 0 || !strings.Contains(errout, "account_conflict") || ownerAPosts.Load() != before || ownerBPosts.Load() != 0 {
		t.Fatalf("account/endpoint conflict: exit %d: %s %s", code, out, errout)
	}
	out, errout, code = run("satellite", "transfer", "alpha", "--to", "next@example.invalid", "--account", "fixture-a", "--no-input", "--json")
	if code == 0 || !strings.Contains(errout, "requires --yes") || ownerAPosts.Load() != before {
		t.Fatalf("noninteractive refusal: exit %d: %s %s", code, out, errout)
	}
	out, errout, code = run("satellite", "transfer", "alpha", "--to", "next@example.invalid", "--account", "fixture-a")
	if code == 0 || !strings.Contains(errout, "requires --yes") || ownerAPosts.Load() != before {
		t.Fatalf("piped stdin refusal: exit %d: %s %s", code, out, errout)
	}
	out, errout, code = run("satellite", "transfer", "alpha", "--to", "next@example.invalid", "--account", "fixture-a", "--yes", "--json")
	if code != 0 || ownerAPosts.Load() != before+1 {
		t.Fatalf("JSON transfer: exit %d: %s %s", code, out, errout)
	}
	var transfer map[string]any
	if json.Unmarshal([]byte(out), &transfer) != nil || !strings.Contains(out, "sat-fixture-a") || !strings.Contains(out, "next@example.invalid") {
		t.Fatalf("invalid transfer JSON: %s", out)
	}
	out, errout, code = run("satellite", "invite", "--account", "fixture-invite", "--name", "fixture-host", "--stdout", "--json")
	if code != 0 || !strings.Contains(out, "fixture-invite-code") || invitePosts.Load() != 2 {
		t.Fatalf("account invite: exit %d: %s %s", code, out, errout)
	}
	if ownerBPosts.Load() != 0 || wrongAuth.Load() != 0 {
		t.Fatalf("cross-account mutation or credential use: posts=%d auth=%d", ownerBPosts.Load(), wrongAuth.Load())
	}
}
