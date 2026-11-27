package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

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
			for _, tool := range []string{"shape_wish", "shape_pull", "shape_push", "shape_verify", "shape_preview", "shape_approve", "shape_undo"} {
				args := map[string]any{"id": "candidate"}
				want := tc.shape
				if tool == "shape_wish" {
					args = map[string]any{"request": "Change the heading"}
					want = tc.wish
				}
				if tool == "shape_approve" || tool == "shape_undo" {
					want = tc.approve
				}
				if tool == "shape_push" {
					args = map[string]any{"base_revision": "base", "patch": "diff"}
				}
				if tool == "shape_undo" || tool == "shape_pull" {
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
				if tool == "shape_push" && (r.Verb != "push" || r.BaseRevision != "base" || r.Patch != "diff") || tool == "shape_pull" && r.Verb != "pull" {
					t.Fatalf("%s dispatch = %+v", tool, r)
				}
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
	if candidate.Kind != shape.KindRequest || candidate.Status != shape.Requested || candidate.Principal != "member@example.com" || candidate.RepresentedUser != "member@example.com" || candidate.ActivationPrincipal != "" || candidate.Workspace != "" || candidate.Actor != "" {
		t.Fatalf("request=%+v", candidate)
	}
	// Opening the owner queue through the core proves the real adapter retained
	// the owner identity. An independent member queue must remain absent.
	svc := shape.Service{App: shape.App{Name: "site"}, Options: shape.Options{Home: home}}
	ownerDir, err := svc.CandidateDir("owner@example.com", candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The member's reply is filtered; the owner's stored record keeps the
	// owner and host identities.
	stored, err := os.ReadFile(filepath.Join(ownerDir, "candidate.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record shape.Candidate
	if err = json.Unmarshal(stored, &record); err != nil {
		t.Fatal(err)
	}
	if record.ActivationPrincipal != "owner@example.com" || record.Actor != "owner-agent" {
		t.Fatalf("stored request=%+v", record)
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

type sitePatchVerifier struct{ calls []shape.VerifyRequest }

func (v *sitePatchVerifier) Verify(_ context.Context, r shape.VerifyRequest) (shape.Verdict, error) {
	v.calls = append(v.calls, r)
	return shape.Verdict{Pass: true}, nil
}

func TestSiteMCPMemberSourceTransfer(t *testing.T) {
	home, source := t.TempDir(), t.TempDir()
	t.Setenv("BASHY_HOME", home)
	stubSiteShape(t)
	siteShapeExecute = executeShapeCLI
	shapeAppLookup = func(string) (shape.App, error) { return shape.App{Name: "site", SourceDir: source}, nil }
	verifier := &sitePatchVerifier{}
	oldOptions := shapeServiceOptions
	t.Cleanup(func() { shapeServiceOptions = oldOptions })
	shapeServiceOptions = func(string) shape.Options { return shape.Options{Home: home, Verifier: verifier} }
	repo, err := gogit.PlainInit(source, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"page": "old\n", "shaped.json": `{"scopes":{"shape":{"groups":["support"]}}}`} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add(name); err != nil {
			t.Fatal(err)
		}
	}
	hash, err := wt.Commit("base", &gogit.CommitOptions{Author: &object.Signature{Name: "owner", Email: "owner@example.test", When: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	writeSiteOutpostConfig(t, "http://127.0.0.1:1", true)
	cs := serveSiteHTTP(t, newSiteMCP(context.Background(), "site", "", true))(func(h http.Header) {
		stampAudience(h, siteTestKey, "member", "member@example.com", "", "contractor", "support")
	})
	pulled, refusal := callSite(t, cs, "shape_pull", nil)
	if refusal != "" {
		t.Fatal(refusal)
	}
	var snapshot shape.SourceSnapshot
	raw, err := json.Marshal(pulled["result"])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != hash.String() || len(snapshot.Files) != 2 || snapshot.Files[0].Path != "page" || string(snapshot.Files[0].Content) != "old\n" {
		t.Fatalf("pull = %+v", snapshot)
	}
	args := map[string]any{"base_revision": snapshot.Revision, "patch": "--- a/page\n+++ b/page\n@@ -1 +1 @@\n-old\n+new\n"}
	pushed, refusal := callSite(t, cs, "shape_push", args)
	if refusal != "" {
		t.Fatal(refusal)
	}
	c, ok := pushed["result"].(map[string]any)
	if !ok || c["status"] != shape.Verified || c["principal"] != "member@example.com" || c["activation_principal"] != nil ||
		c["base_rev"] != snapshot.Revision || len(verifier.calls) != 1 || verifier.calls[0].Principal != "owner@example.com" || verifier.calls[0].CandidateRev != c["rev"] {
		t.Fatalf("push = %v, verifier = %+v", c, verifier.calls)
	}
	args["base_revision"] = "stale"
	if _, refusal := callSite(t, cs, "shape_push", args); !strings.Contains(refusal, "base stale") || !strings.Contains(refusal, "active "+snapshot.Revision) || len(verifier.calls) != 1 {
		t.Fatalf("stale push: refusal=%q verifier calls=%d", refusal, len(verifier.calls))
	}
}

// A caller who is not the owner gets the member view of a candidate: no host
// path, board item, host actor or owner identity; the owner still gets all.
func TestSiteMCPMemberReplyHasNoHostFields(t *testing.T) {
	stubSiteShape(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shaped.json"), []byte(`{"scopes":{"shape":{"groups":["support"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	shapeAppLookup = func(string) (shape.App, error) { return shape.App{Name: "site", SourceDir: dir}, nil }
	host := filepath.Join(t.TempDir(), "host")
	siteShapeExecute = func(_ context.Context, r shapeCLIRequest) (any, error) {
		c := shape.Candidate{ID: "0123456789abcdef", App: "site", Principal: r.User, Status: shape.Verified, Rev: "rev1",
			Workspace: filepath.Join(host, "work"), TrackingRoot: filepath.Join(host, "todo"), TrackingItem: "T-42",
			ArtifactPath: filepath.Join(host, "artifact"), Actor: r.Actor, ActivationPrincipal: "owner@example.com",
			Identity: "agent-identity", Receipt: "agent-receipt",
			Verdict: &shape.Verdict{Pass: true, Evidence: filepath.Join(host, "src", ".evidence", "rev1.json")}}
		if r.Verb == "preview" {
			return map[string]any{"app": r.App, "principal": r.User, "candidate": c, "url": "http://127.0.0.1:1/shape/site/preview/" + c.ID + "/"}, nil
		}
		return c, nil
	}
	writeSiteOutpostConfig(t, "http://127.0.0.1:1", false)
	connect := serveSiteHTTP(t, newSiteMCP(context.Background(), "site", "", true))
	leaks := []string{host, "T-42", "site-owner-agent", "owner@example.com", "agent-identity", "agent-receipt"}
	for _, tier := range []string{"member", "owner"} {
		cs := connect(func(h http.Header) {
			stampAudience(h, siteTestKey, tier, tier+"@example.net", "", "company", "support")
		})
		for tool, args := range map[string]map[string]any{
			"shape_wish":    {"request": "Change the heading"},
			"shape_verify":  {"id": "0123456789abcdef"},
			"shape_preview": {"id": "0123456789abcdef"},
		} {
			out, refusal := callSite(t, cs, tool, args)
			if refusal != "" {
				t.Fatalf("%s %s: %s", tier, tool, refusal)
			}
			data, _ := json.Marshal(out["result"])
			for _, leak := range leaks {
				if got := strings.Contains(string(data), leak); got != (tier == "owner") {
					t.Errorf("%s %s: reply contains %q = %v: %s", tier, tool, leak, got, data)
				}
			}
			candidate := out["result"].(map[string]any)
			if tool == "shape_preview" {
				if candidate["url"] == "" {
					t.Errorf("%s preview lost its url", tier)
				}
				candidate = candidate["candidate"].(map[string]any)
			}
			if candidate["id"] != "0123456789abcdef" || candidate["rev"] != "rev1" || candidate["status"] != shape.Verified {
				t.Errorf("%s %s: candidate = %v", tier, tool, candidate)
			}
			if tier == "member" && candidate["verdict"].(map[string]any)["evidence"] != "shape-evidence:rev1" {
				t.Errorf("member %s: verdict = %v", tool, candidate["verdict"])
			}
		}
	}
}

// A member's push reply carries no owner identity, and a failed verification
// tells a member its detail without the host's paths; the owner sees both.
func TestSiteMCPMemberPushAndFailedVerifyHaveNoHostFields(t *testing.T) {
	stubSiteShape(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shaped.json"), []byte(`{"scopes":{"shape":{"groups":["support"]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	shapeAppLookup = func(string) (shape.App, error) { return shape.App{Name: "site", SourceDir: dir}, nil }
	host := filepath.Join(t.TempDir(), "host")
	work := filepath.Join(host, "work")
	siteShapeExecute = func(_ context.Context, r shapeCLIRequest) (any, error) {
		if r.Verb == "push" {
			return map[string]string{"id": "0123456789abcdef", "status": shape.Verified, "principal": r.User,
				"activation_principal": "owner@example.com", "base_rev": "rev0", "rev": "rev1"}, nil
		}
		c := shape.Candidate{ID: "0123456789abcdef", App: "site", Principal: r.User, Status: shape.Ready, Rev: "rev1", Workspace: work,
			Verdict: &shape.Verdict{Pass: false, Detail: "check failed in " + filepath.Join(work, "checks", "a.bsh")}}
		return c, fmt.Errorf("shape: verification failed: %s", c.Verdict.Detail)
	}
	writeSiteOutpostConfig(t, "http://127.0.0.1:1", false)
	connect := serveSiteHTTP(t, newSiteMCP(context.Background(), "site", "", true))
	for _, tier := range []string{"member", "owner"} {
		cs := connect(func(h http.Header) {
			stampAudience(h, siteTestKey, tier, tier+"@example.net", "", "company", "support")
		})
		out, refusal := callSite(t, cs, "shape_push", map[string]any{"base_revision": "rev0", "patch": "x"})
		if refusal != "" {
			t.Fatalf("%s push: %s", tier, refusal)
		}
		data, _ := json.Marshal(out["result"])
		if got := strings.Contains(string(data), "owner@example.com"); got != (tier == "owner") {
			t.Errorf("%s push reply contains owner identity = %v: %s", tier, got, data)
		}
		_, refusal = callSite(t, cs, "shape_verify", map[string]any{"id": "0123456789abcdef"})
		if !strings.Contains(refusal, "verification failed") || !strings.Contains(refusal, "a.bsh") {
			t.Fatalf("%s verify refusal lost its meaning: %q", tier, refusal)
		}
		if got := strings.Contains(refusal, host); got != (tier == "owner") {
			t.Errorf("%s verify refusal contains host path = %v: %q", tier, got, refusal)
		}
	}
}
