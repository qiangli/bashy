package agentos

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/shape"
)

func TestSiteMCPScopeMatrix(t *testing.T) {
	calls := stubSiteShape(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shaped.json"), []byte(`{"scopes":{"shape":{"circles":["contractor"],"groups":["support"]},"edit":{"circles":["company"],"groups":["support"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	shapeAppLookup = func(string) (shape.App, error) { return shape.App{Name: "site", SourceDir: dir}, nil }
	writeSiteOutpostConfig(t, "http://127.0.0.1:1", false)
	connect := serveSiteHTTP(t, newSiteMCP(context.Background(), "site", "", true))
	for _, tc := range []struct {
		name, tier, circle, groups   string
		wish, shape, approve, tamper bool
	}{
		{name: "owner", tier: "owner", wish: true, shape: true, approve: true},
		{name: "member shape", tier: "member", circle: "contractor", groups: "support,extra", wish: true, shape: true},
		{name: "member edit", tier: "member", circle: "company", groups: "support", wish: true},
		{name: "member no scope", tier: "member", circle: "contractor", groups: "other", wish: true},
		{name: "account with matching audience", tier: "account", circle: "contractor", groups: "support"},
		{name: "anonymous"},
		{name: "tampered audience", tier: "member", circle: "company", groups: "support", tamper: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := "member@example.com"
			if tc.tier == "owner" {
				user = "owner@example.com"
			}
			cs := connect(func(h http.Header) {
				if tc.tier != "" {
					stampAudience(h, siteTestKey, tc.tier, user, "", tc.circle, tc.groups)
				}
				if tc.tamper {
					h.Set("X-Outpost-Circle", "contractor")
				}
			})
			for _, tool := range []string{"shape_wish", "shape_verify", "shape_preview", "shape_approve", "shape_undo"} {
				args := map[string]any{"id": "candidate"}
				want := tc.shape
				if tool == "shape_wish" {
					args = map[string]any{"request": "Change the heading"}
					want = tc.wish
				}
				if tool == "shape_approve" || tool == "shape_undo" {
					want = tc.approve
				}
				if tool == "shape_undo" {
					args = nil
				}
				before := len(*calls)
				_, refusal := callSite(t, cs, tool, args)
				if (refusal == "") != want {
					t.Fatalf("%s refusal=%s want allowed=%v", tool, refusal, want)
				}
				if !want {
					if len(*calls) != before {
						t.Fatal("refused tool executed")
					}
					continue
				}
				r := (*calls)[before]
				if r.Owner != "owner@example.com" || r.Caller == nil || r.Caller.Principal != user || r.Caller.Circle != tc.circle || r.Caller.Tier != tc.tier {
					t.Fatalf("lost trusted caller: %+v", r)
				}
				if tc.groups != "" && (len(r.Caller.Groups) == 0 || r.Caller.Groups[0] != "support" && tc.groups != "other") {
					t.Fatalf("lost groups: %+v", r.Caller)
				}
			}
		})
	}
}

func TestSiteMCPUnscopedWishUsesOwnerQueue(t *testing.T) {
	home, source := t.TempDir(), t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Chdir(home)
	oldLookup, oldOptions, oldRoot, oldActor := shapeAppLookup, shapeServiceOptions, shapeTrackingRoot, shapeAuthoredActor
	t.Cleanup(func() {
		shapeAppLookup, shapeServiceOptions, shapeTrackingRoot, shapeAuthoredActor = oldLookup, oldOptions, oldRoot, oldActor
	})
	shapeAppLookup = func(string) (shape.App, error) { return shape.App{Name: "site", SourceDir: source}, nil }
	shapeServiceOptions = func(string) shape.Options { return shape.Options{Home: home} }
	shapeTrackingRoot = func(string) (string, error) { return filepath.Join(home, "tracking"), nil }
	shapeAuthoredActor = func(string) (string, error) { return "owner-agent", nil }
	writeSiteOutpostConfig(t, "http://127.0.0.1:1", false)
	connect := serveSiteHTTP(t, newSiteMCP(context.Background(), "site", "", true))
	cs := connect(func(h http.Header) {
		stampAudience(h, siteTestKey, "member", "member@example.com", "", "company", "support")
	})
	out, refusal := callSite(t, cs, "shape_wish", map[string]any{"request": "Change the header", "page": "/about"})
	if refusal != "" {
		t.Fatal(refusal)
	}
	data, err := json.Marshal(out["result"])
	if err != nil {
		t.Fatal(err)
	}
	var candidate shape.Candidate
	if err = json.Unmarshal(data, &candidate); err != nil {
		t.Fatal(err)
	}
	if candidate.Kind != shape.KindRequest || candidate.Status != shape.Requested || candidate.Principal != "member@example.com" || candidate.RepresentedUser != "member@example.com" || candidate.ActivationPrincipal != "owner@example.com" || candidate.Workspace != "" || candidate.Actor != "owner-agent" {
		t.Fatalf("request=%+v", candidate)
	}
	// Opening the owner queue through the core proves the real adapter retained
	// the owner identity. An independent member queue must remain absent.
	svc := shape.Service{App: shape.App{Name: "site"}, Options: shape.Options{Home: home}}
	ownerDir, err := svc.CandidateDir("owner@example.com", candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(ownerDir, "candidate.json")); err != nil {
		t.Fatal(err)
	}
	memberDir, err := svc.CandidateDir("member@example.com", candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(memberDir); !os.IsNotExist(err) {
		t.Fatalf("member queue exists: %v", err)
	}
}

func TestSiteMCPMemberRequiresConfiguredOwner(t *testing.T) {
	calls := stubSiteShape(t)
	writeSiteOutpostConfig(t, "http://127.0.0.1:1", false)
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "matrix", "agent.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err = json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	delete(cfg, "oauth2_email")
	data, err = json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cs := serveSiteHTTP(t, newSiteMCP(context.Background(), "site", "", true))(func(h http.Header) { stamp(h, siteTestKey, "member", "member@example.com", "") })
	_, refusal := callSite(t, cs, "shape_wish", map[string]any{"request": "Change the heading"})
	if !strings.Contains(refusal, "owner identity is not configured") || len(*calls) != 0 {
		t.Fatalf("missing owner: refusal=%q calls=%+v", refusal, *calls)
	}
}
