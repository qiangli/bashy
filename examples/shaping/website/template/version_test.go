package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

func TestVersionEndpointAndMeReportVersion(t *testing.T) {
	s := newTestServer(t, "http://127.0.0.1:1", "http://127.0.0.1:1", "k")

	// Default version comes from embedded/fixture assets digest and is non-empty.
	code, verResp := do(t, s, "GET", "/_shaping/api/version", nobody)
	if code != http.StatusOK {
		t.Fatalf("GET /_shaping/api/version: status %d", code)
	}
	v, _ := verResp["version"].(string)
	if v == "" {
		t.Fatalf("GET /_shaping/api/version: empty version: %v", verResp)
	}

	// /_shaping/api/me exposes the exact same version id for every tier.
	for _, w := range []who{owner, recruiter, stranger, nobody} {
		_, me := do(t, s, "GET", "/_shaping/api/me", w)
		if got, _ := me["version"].(string); got != v {
			t.Errorf("%s: me.version = %q, want %q", w.name, got, v)
		}
	}

	// Cache-Control must be no-store so proxies and browsers never cache stale ids.
	req := httptest.NewRequest("GET", "/_shaping/api/version", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	// Non-GET/HEAD returns 405.
	if code, _ := do(t, s, "POST", "/_shaping/api/version", nobody); code != http.StatusMethodNotAllowed {
		t.Errorf("POST /_shaping/api/version: status %d, want 405", code)
	}

	// SITE_VERSION overrides the computed digest.
	env := map[string]string{"SITE_SSO_SECRET": testSecret, "SITE_VERSION": "rev-abc123"}
	s2, err := newServerFrom(fixture, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	_, verResp2 := do(t, s2, "GET", "/_shaping/api/version", nobody)
	if got, _ := verResp2["version"].(string); got != "rev-abc123" {
		t.Errorf("SITE_VERSION override: got %q, want rev-abc123", got)
	}

	// Changing an asset changes the fallback digest.
	modified := fstest.MapFS{}
	for k, f := range fixture {
		modified[k] = f
	}
	modified["fields/notes.md"] = &fstest.MapFile{Data: []byte("---\nlabel: Owner notes\n---\nupdated\n")}
	if computeAssetsVersion(modified) == v {
		t.Error("computeAssetsVersion did not change when an asset changed")
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Owner",
		"GIT_AUTHOR_EMAIL=owner@example.com",
		"GIT_COMMITTER_NAME=Owner",
		"GIT_COMMITTER_EMAIL=owner@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func copyTemplateTree(t *testing.T, dst string) {
	t.Helper()
	src, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".build" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeShapeState(t *testing.T, home, app, principal, activeRev, lastKnownGood string) {
	t.Helper()
	dir := filepath.Join(home, "shape", encodeStateComponent(app), encodeStateComponent(principal))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := json.MarshalIndent(map[string]string{
		"schema":          "bashy-shape-state-v1",
		"active_revision": activeRev,
		"last_known_good": lastKnownGood,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, ".state.tmp")
	if err := os.WriteFile(tmp, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, "state.json")); err != nil {
		t.Fatal(err)
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func fetchJSON(url string) (map[string]any, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// TestApproveOnHostRebuildsAndChangesServedVersion proves Step 1 end-to-end:
// a running site-shaping server watches the host's shaping state for the
// owner's active revision, rebuilds from that commit in the managed src
// checkout when an approval updates state.json (even when the checkout's
// working tree stays on the base revision), and serves the new version id
// and content on the same address.
func TestApproveOnHostRebuildsAndChangesServedVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("in-place process swap test targets POSIX exec")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}

	root := t.TempDir()
	home := filepath.Join(root, "bashy-home")
	app := "site-shaping"
	srcDir := filepath.Join(home, "src", app)
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyTemplateTree(t, srcDir)

	// Revision 1: initial title.
	siteJSONPath := filepath.Join(srcDir, "site.json")
	if err := os.WriteFile(siteJSONPath, []byte(`{"site_url":"http://127.0.0.1:1","title":"Version One"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, srcDir, "init", "-q")
	runGit(t, srcDir, "add", ".")
	runGit(t, srcDir, "commit", "-q", "-m", "rev1")
	rev1 := runGit(t, srcDir, "rev-parse", "HEAD")

	// Write initial state at rev1.
	writeShapeState(t, home, app, "owner@example.com", rev1, "")

	// Build initial binary from srcDir.
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(binDir, "site-shaping")
	build := exec.Command("go", "build", "-trimpath", "-o", bin, ".")
	build.Dir = srcDir
	build.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("initial build: %v\n%s", err, out)
	}

	addr := freePort(t)
	cmd := exec.Command(bin, "-addr", addr)
	cmd.Env = append(os.Environ(),
		"SITE_SSO_SECRET="+testSecret,
		"SITE_SHAPE_APP="+app,
		"SITE_SHAPE_HOME="+home,
		"SITE_SHAPE_PRINCIPAL=owner@example.com",
		"SITE_VERSION="+rev1,
		"SITE_FOLLOW_INTERVAL=100ms",
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start site-shaping: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	baseURL := "http://" + addr
	// Wait for rev1 to answer.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if v, err := fetchJSON(baseURL + "/_shaping/api/version"); err == nil {
			if got, _ := v["version"].(string); got != rev1 {
				t.Fatalf("initial served version = %q, want %q", got, rev1)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("site-shaping did not start serving rev1 in time")
		}
		time.Sleep(50 * time.Millisecond)
	}
	me1, err := fetchJSON(baseURL + "/_shaping/api/me")
	if err != nil {
		t.Fatalf("GET /_shaping/api/me before approve: %v", err)
	}
	if me1["title"] != "Version One" || me1["version"] != rev1 {
		t.Fatalf("before approve: me = %v", me1)
	}

	// Simulate an approval on the host: commit rev2 in the managed checkout,
	// then point HEAD/worktree back to rev1 (mirroring src.Activate, which
	// keeps the approved commit in git history without touching the worktree),
	// and atomically write state.json with active_revision = rev2.
	if err := os.WriteFile(siteJSONPath, []byte(`{"site_url":"http://127.0.0.1:1","title":"Version Two"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, srcDir, "add", "site.json")
	runGit(t, srcDir, "commit", "-q", "-m", "rev2")
	rev2 := runGit(t, srcDir, "rev-parse", "HEAD")
	runGit(t, srcDir, "update-ref", "refs/shaping/owner-example-com/line", rev2)
	runGit(t, srcDir, "reset", "--hard", "-q", rev1)

	// A member's own (per-person) activation must never become the served site.
	writeShapeState(t, home, app, "member@example.com", rev2, rev1)
	time.Sleep(1 * time.Second)
	if v, err := fetchJSON(baseURL + "/_shaping/api/version"); err == nil {
		if got, _ := v["version"].(string); got != rev1 {
			t.Fatalf("served version followed a member's activation: got %s want %s", got, rev1)
		}
	}

	writeShapeState(t, home, app, "owner@example.com", rev2, rev1)

	// Wait for the running server to rebuild from rev2 and swap on the same address.
	swapDeadline := time.Now().Add(30 * time.Second)
	for {
		if v, err := fetchJSON(baseURL + "/_shaping/api/version"); err == nil {
			if got, _ := v["version"].(string); got == rev2 {
				break
			}
		}
		if time.Now().After(swapDeadline) {
			t.Fatalf("served version did not update from %s to %s after approve", shortRev(rev1), shortRev(rev2))
		}
		time.Sleep(150 * time.Millisecond)
	}

	me2, err := fetchJSON(baseURL + "/_shaping/api/me")
	if err != nil {
		t.Fatalf("GET /_shaping/api/me after approve: %v", err)
	}
	if me2["title"] != "Version Two" {
		t.Errorf("after approve: title = %v, want Version Two", me2["title"])
	}
	if me2["version"] != rev2 {
		t.Errorf("after approve: version = %v, want %s", me2["version"], rev2)
	}
}

// TestShapingJSVersionPollReloadsOnChangeAndNoLoopWhenUnchanged runs the real
// public/shaping.js in a minimal DOM harness against a live httptest.Server
// whose served version id changes when approved on the host. It verifies:
//  1. Unchanged version id across repeated polls causes zero reloads (no loop).
//  2. Changed version id triggers exactly one reload when the viewer is idle.
//  3. Changed version id while the viewer is editing shows a one-line notice
//     instead of reloading, and clicking its Reload button reloads.
//  4. A transient 502/network error during restart does not break polling;
//     the next poll sees the new version id and reloads.
func TestShapingJSVersionPollReloadsOnChangeAndNoLoopWhenUnchanged(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}

	var curVersion atomic.Value
	curVersion.Store("rev-111111111111")
	var failVersion atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_shaping/api/me":
			writeJSON(w, http.StatusOK, map[string]any{
				"tier":    "anonymous",
				"title":   "Test Site",
				"sign_in": "/cloud/signin",
				"fields":  0,
				"more":    false,
				"views":   []any{},
				"version": curVersion.Load().(string),
				"public":  true,
			})
		case "/_shaping/api/version":
			if failVersion.Load() {
				http.Error(w, "restarting", http.StatusBadGateway)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"version": curVersion.Load().(string)})
		case "/_test/set-version":
			curVersion.Store(r.URL.Query().Get("v"))
			failVersion.Store(r.URL.Query().Get("fail") == "1")
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(wd, "public", "shaping.js")

	const harness = `
const fs = require("fs");
const vm = require("vm");
const assert = require("assert");

const scriptPath = process.argv[1];
const baseURL = process.argv[2];
const code = fs.readFileSync(scriptPath, "utf8");

function makeNode(tag, ns) {
  const attrs = {};
  const listeners = {};
  const node = {
    tagName: (tag || "").toUpperCase(),
    namespaceURI: ns || null,
    children: [],
    parentNode: null,
    value: "",
    isContentEditable: false,
    disabled: false,
    _text: "",
    get textContent() {
      return this._text + this.children.map(c => c.textContent).join("");
    },
    set textContent(v) {
      this._text = String(v == null ? "" : v);
      this.children = [];
    },
    get firstChild() { return this.children[0] || null; },
    setAttribute(k, v) { attrs[k] = String(v); },
    getAttribute(k) { return Object.prototype.hasOwnProperty.call(attrs, k) ? attrs[k] : null; },
    hasAttribute(k) { return Object.prototype.hasOwnProperty.call(attrs, k); },
    removeAttribute(k) { delete attrs[k]; },
    appendChild(c) { c.parentNode = this; this.children.push(c); return c; },
    insertBefore(c, ref) {
      c.parentNode = this;
      const i = this.children.indexOf(ref);
      if (i >= 0) this.children.splice(i, 0, c);
      else this.children.push(c);
      return c;
    },
    remove() {
      if (!this.parentNode) return;
      const i = this.parentNode.children.indexOf(this);
      if (i >= 0) this.parentNode.children.splice(i, 1);
      this.parentNode = null;
    },
    addEventListener(ev, fn) { (listeners[ev] = listeners[ev] || []).push(fn); },
    click() { (listeners.click || []).forEach(fn => fn({ preventDefault() {} })); },
    querySelectorAll(sel) {
      const want = sel.split(",").map(s => s.trim());
      const out = [];
      (function walk(n) {
        for (const c of n.children) {
          for (const w of want) {
            if (w.startsWith(".") && (c.getAttribute("class") || "").split(/\s+/).includes(w.slice(1))) out.push(c);
            else if (c.tagName && c.tagName.toLowerCase() === w.toLowerCase()) out.push(c);
          }
          walk(c);
        }
      })(this);
      return out;
    },
    querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
  };
  return node;
}

async function flush() {
  for (let i = 0; i < 8; i++) await new Promise(r => setImmediate(r));
}

async function setServer(v, fail) {
  const r = await fetch(baseURL + "/_test/set-version?v=" + encodeURIComponent(v) + "&fail=" + (fail ? "1" : "0"));
  assert.strictEqual(r.status, 204);
}

async function openPage() {
  const body = makeNode("body");
  let reloads = 0;
  let intervalFn = null;
  let intervalMs = 0;
  let cleared = false;
  const docListeners = {};
  const document = {
    body,
    cookie: "",
    hidden: false,
    activeElement: null,
    currentScript: { src: baseURL + "/_shaping/shaping.js" },
    createElement: (tag) => makeNode(tag),
    createElementNS: (ns, tag) => makeNode(tag, ns),
    createTextNode: (t) => { const n = makeNode("#text"); n._text = String(t); return n; },
    getElementById(id) {
      let found = null;
      (function walk(n) {
        if (found) return;
        for (const c of n.children) {
          if (c.getAttribute("id") === id) { found = c; return; }
          walk(c);
        }
      })(body);
      return found;
    },
    querySelector: (s) => body.querySelector(s),
    querySelectorAll: (s) => body.querySelectorAll(s),
    addEventListener(ev, fn) { (docListeners[ev] = docListeners[ev] || []).push(fn); }
  };
  const sandbox = {
    document,
    location: { origin: baseURL, pathname: "/", search: "", reload() { reloads++; } },
    URL,
    fetch,
    setInterval(fn, ms) { intervalFn = fn; intervalMs = ms; cleared = false; return 1; },
    clearInterval() { cleared = true; intervalFn = null; },
    console
  };
  vm.runInNewContext(code, sandbox, { filename: scriptPath });
  // Wait for initial GET /_shaping/api/me to mount the panel and arm polling.
  const deadline = Date.now() + 5000;
  while (!intervalFn && Date.now() < deadline) {
    await new Promise(r => setTimeout(r, 10));
  }
  assert.ok(intervalFn, "panel did not start version polling");
  assert.ok(intervalMs >= 1000 && intervalMs <= 10000, "unexpected poll interval: " + intervalMs);
  return {
    document,
    get reloads() { return reloads; },
    get cleared() { return cleared; },
    async tick() {
      if (intervalFn) intervalFn();
      await new Promise(r => setTimeout(r, 30));
      await flush();
    }
  };
}

(async () => {
  // 1. Unchanged id: repeated polls never reload (no reload loop).
  await setServer("rev-111111111111", false);
  const p1 = await openPage();
  for (let i = 0; i < 4; i++) await p1.tick();
  assert.strictEqual(p1.reloads, 0, "reloaded when version was unchanged");
  assert.strictEqual(p1.document.querySelector(".ss-update-notice"), null, "notice shown when version unchanged");

  // 2. Approve changes served id -> page holding old id reloads exactly once.
  await setServer("rev-222222222222", false);
  await p1.tick();
  assert.strictEqual(p1.reloads, 1, "did not reload when version changed");
  assert.strictEqual(p1.cleared, true, "poll timer not stopped after reload");
  await p1.tick();
  assert.strictEqual(p1.reloads, 1, "reloaded more than once");

  // 3. While viewer is editing, show a one-line notice instead of reloading;
  //    clicking Reload reloads.
  await setServer("rev-222222222222", false);
  const p2 = await openPage();
  const textarea = p2.document.createElement("textarea");
  p2.document.body.appendChild(textarea);
  p2.document.activeElement = textarea;
  await setServer("rev-333333333333", false);
  await p2.tick();
  assert.strictEqual(p2.reloads, 0, "reloaded while viewer was editing");
  const notices = p2.document.querySelectorAll(".ss-update-notice");
  assert.strictEqual(notices.length, 1, "expected one update notice while editing");
  assert.ok(/new version/i.test(notices[0].textContent), "unexpected notice text: " + notices[0].textContent);
  await p2.tick();
  assert.strictEqual(p2.document.querySelectorAll(".ss-update-notice").length, 1, "duplicate notice shown");
  const btn = notices[0].querySelector(".ss-reload");
  assert.ok(btn, "notice missing Reload button");
  btn.click();
  assert.strictEqual(p2.reloads, 1, "clicking Reload button did not reload");

  // 4. Transient 502 during restart does not break polling; next tick reloads.
  await setServer("rev-333333333333", false);
  const p3 = await openPage();
  await setServer("rev-444444444444", true);
  await p3.tick();
  assert.strictEqual(p3.reloads, 0, "reloaded during 502");
  await setServer("rev-444444444444", false);
  await p3.tick();
  assert.strictEqual(p3.reloads, 1, "did not reload after server came back with new version");
})().catch(err => { console.error(err); process.exit(1); });
`

	cmd := exec.Command(nodeBin, "-e", harness, "--", scriptPath, srv.URL)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("DOM version polling test failed: %v\n%s", err, out)
	}
}
