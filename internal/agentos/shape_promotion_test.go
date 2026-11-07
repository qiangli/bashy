package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/qiangli/yoke/pkg/shape"
)

func promotionFixture(t *testing.T) (bare string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	source := t.TempDir()
	repo, err := gogit.PlainInit(source, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	wt, _ := repo.Worktree()
	_, _ = wt.Add("a.txt")
	if _, err = wt.Commit("base", &gogit.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.invalid", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	bare = t.TempDir()
	if _, err = gogit.PlainInit(bare, true); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.CreateRemote(&config.RemoteConfig{Name: "seed", URLs: []string{bare}}); err != nil {
		t.Fatal(err)
	}
	if err = repo.Push(&gogit.PushOptions{RemoteName: "seed", RefSpecs: []config.RefSpec{"refs/heads/*:refs/heads/*"}}); err != nil {
		t.Fatal(err)
	}
	oldLookup, oldOptions := shapeAppLookup, shapeServiceOptions
	t.Cleanup(func() { shapeAppLookup = oldLookup; shapeServiceOptions = oldOptions })
	shapeAppLookup = func(string) (shape.App, error) {
		return shape.App{Name: "bashy", SourceDir: source}, nil
	}
	shapeServiceOptions = func(string) shape.Options { return shape.ProductionOptions(home) }
	return bare
}

func runPromotion(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	cmd := bashyShapeCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	code := 0
	if err := cmd.Execute(); err != nil {
		code = 1
		errb.WriteString(err.Error())
		if err == errShapePending {
			code = 3
		}
	}
	return code, out.String(), errb.String()
}

func TestPromotionRemoteSetShowJSON(t *testing.T) {
	bare := promotionFixture(t)
	if code, _, _ := runPromotion(t, "remote", "set", "org", bare); code != 0 {
		t.Fatalf("set exit %d", code)
	}
	code, out, _ := runPromotion(t, "remote", "show", "--json")
	if code != 0 {
		t.Fatalf("show exit %d", code)
	}
	var env struct {
		SchemaVersion string            `json:"schema_version"`
		Remotes       map[string]string `json:"remotes"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if env.SchemaVersion != "bashy-shape-promotion-v1" || env.Remotes["org"] != bare {
		t.Fatalf("envelope=%+v", env)
	}
	if code, _, _ := runPromotion(t, "remote", "set", "mars", bare); code != 1 {
		t.Fatalf("bad tier exit %d", code)
	}
}

func TestPromotionOutgoingExitCodes(t *testing.T) {
	bare := promotionFixture(t)
	if code, _, _ := runPromotion(t, "outgoing", "--to", "org"); code != 1 {
		t.Fatalf("unconfigured tier exit %d, want 1", code)
	}
	if code, _, _ := runPromotion(t, "remote", "set", "org", bare); code != 0 {
		t.Fatal("set failed")
	}
	code, out, errs := runPromotion(t, "outgoing", "--to", "org", "--json")
	if code != 0 {
		t.Logf("stderr=%s", errs)
		t.Fatalf("empty outgoing exit %d, want 0: %s", code, out)
	}
	if !strings.Contains(out, `"schema_version": "bashy-shape-promotion-v1"`) || !strings.Contains(out, `"pending": 0`) {
		t.Fatalf("json=%s", out)
	}
	if code, _, _ := runPromotion(t, "outgoing", "--to", "nowhere"); code != 1 {
		t.Fatalf("bad tier exit %d", code)
	}
	if code, _, _ := runPromotion(t, "push"); code != 1 {
		t.Fatalf("push without --to exit %d", code)
	}
	if code, out, _ := runPromotion(t, "push", "--to", "org"); code != 0 || !strings.Contains(out, "nothing to push") {
		t.Fatalf("empty push exit %d out=%q", code, out)
	}
}

func TestPromotionPendingExitsThree(t *testing.T) {
	bare := promotionFixture(t)
	_, _, _ = runPromotion(t, "remote", "set", "org", bare)
	old := shapeOutgoingHook
	shapeOutgoingHook = func(string, string, bool) ([]shapePromotionChange, error) {
		return []shapePromotionChange{{OutgoingChange: shape.OutgoingChange{ID: "x", Title: "t", Status: map[string]string{"org": "pending"}}}}, nil
	}
	defer func() { shapeOutgoingHook = old }()
	code, out, _ := runPromotion(t, "outgoing", "--to", "org", "--json")
	if code != 3 || !strings.Contains(out, `"pending": 1`) {
		t.Fatalf("exit %d out=%s", code, out)
	}
}

func TestPromotionStatusLineUnconfigured(t *testing.T) {
	promotionFixture(t)
	line := shapePromotionStatusLine(t.Context(), "bashy")
	if !strings.Contains(line, "promotion: unconfigured (org)") || !strings.Contains(line, "incoming: unconfigured") {
		t.Fatalf("line=%q", line)
	}
}

func TestPromotionIncomingPullRelease(t *testing.T) {
	bare := promotionFixture(t)
	home := os.Getenv("BASHY_HOME")
	shapeServiceOptions = func(string) shape.Options {
		o := shape.ProductionOptions(home)
		o.InstalledVersion = "v0.0.1"
		return o
	}
	if code, _, _ := runPromotion(t, "incoming", "--from", "org"); code != 1 {
		t.Fatalf("unconfigured incoming exit %d, want 1", code)
	}
	if code, _, _ := runPromotion(t, "remote", "set", "org", bare); code != 0 {
		t.Fatal("set failed")
	}
	if code, out, _ := runPromotion(t, "incoming", "--from", "org", "--json"); code != 0 || !strings.Contains(out, "bashy-shape-promotion-v1") {
		t.Fatalf("empty incoming exit %d out=%s", code, out)
	}
	br, err := gogit.PlainOpen(bare)
	if err != nil {
		t.Fatal(err)
	}
	head, err := br.Head()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = br.CreateTag("v9.9.9", head.Hash(), nil); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runPromotion(t, "incoming", "--from", "org", "--json")
	if code != 3 || !strings.Contains(out, `"kind": "release"`) {
		t.Fatalf("release incoming exit %d out=%s", code, out)
	}
	code, out, errs := runPromotion(t, "pull", "--from", "org", "refs/tags/v9.9.9")
	if code != 0 || !strings.Contains(out, "release v9.9.9") || !strings.Contains(out, "bashy upgrade") {
		t.Fatalf("pull release exit %d out=%q err=%q", code, out, errs)
	}
	if code, _, _ := runPromotion(t, "pull", "--from", "org", "nosuchid"); code != 1 {
		t.Fatalf("unknown ref exit %d, want 1", code)
	}
	if code, _, _ := runPromotion(t, "pull", "refs/tags/v9.9.9"); code != 1 {
		t.Fatalf("pull without --from exit %d, want 1", code)
	}
}
