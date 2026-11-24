package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/bashy/examples/desk/kit"
)

type noteTestCaller struct {
	relationship, user, visitor, address string
	badSignature                         bool
	cookie                               *http.Cookie
}

func (caller noteTestCaller) request(method, path string, body []byte, secret string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:5010"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "notepad-test")
	if caller.cookie != nil {
		req.AddCookie(caller.cookie)
	}
	if caller.relationship != "" {
		const prefix = "/desk/notepad"
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		fields := []string{"v2", caller.relationship, "", "", caller.visitor, caller.user, prefix, ts}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(strings.Join(fields, "\n")))
		signature := hex.EncodeToString(mac.Sum(nil))
		if caller.badSignature {
			signature = strings.Repeat("0", sha256.Size*2)
		}
		req.Header.Set("X-Forwarded-Prefix", prefix)
		req.Header.Set("X-Forwarded-For", caller.address)
		req.Header.Set("X-Outpost-Identity-Ts", ts)
		req.Header.Set("X-Outpost-Relationship", caller.relationship)
		req.Header.Set("X-Outpost-Relationship-Sig", signature)
		req.Header.Set("X-Outpost-Visitor", caller.visitor)
		req.Header.Set("Remote-User", caller.user)
	}
	return req
}

func noteTestAction(t *testing.T, a *app, caller noteTestCaller, input string, status int) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	a.ServeHTTP(w, caller.request(http.MethodPost, "/api/action", []byte(input), a.outpostSecret))
	if w.Code != status {
		t.Fatalf("%s: HTTP %d, want %d: %s", input, w.Code, status, w.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode %s: %v", w.Body.String(), err)
	}
	return w, data
}

func TestNoteRevisionAdmissionAndExport(t *testing.T) {
	t.Setenv("CHROMIUM_PATH", filepath.Join(t.TempDir(), "no-browser"))
	a := &app{dir: t.TempDir(), notes: map[string]*note{}, outpostSecret: "note-test-secret", consolePrincipal: "os:local"}
	owner := noteTestCaller{relationship: "owner", user: "owner@example.test", address: "192.0.2.1"}
	member := noteTestCaller{relationship: "member", user: "member@example.test", address: "192.0.2.2"}
	guest := noteTestCaller{relationship: "anonymous", address: "192.0.2.3"}
	outsider := noteTestCaller{relationship: "account", visitor: "0123456789abcdef0123456789abcdef", address: "192.0.2.4"}
	legacyOutsider := noteTestCaller{relationship: "signed-in", visitor: "fedcba9876543210fedcba9876543210", address: "192.0.2.6"}
	bad := member
	bad.badSignature = true
	for _, c := range []noteTestCaller{guest, outsider, bad} {
		noteTestAction(t, a, c, `{"action":"add","id":"blocked"}`, 403)
	}

	noteTestAction(t, a, owner, `{"action":"add","id":"group","title":"Group note","body":"Owner first version"}`, 200)
	noteTestAction(t, a, member, `{"action":"set","id":"group","base_rev":"r1","body":"Member second version"}`, 200)
	_, conflict := noteTestAction(t, a, owner, `{"action":"set","id":"group","base_rev":"r1","body":"Stale content"}`, 409)
	if conflict["base_rev"] != "r1" || conflict["active"] != "r2" {
		t.Fatalf("stale response lacks both revisions: %v", conflict)
	}
	noteTestAction(t, a, member, `{"action":"share","id":"group"}`, 403)
	for _, c := range []noteTestCaller{guest, outsider, bad} {
		for _, verb := range []string{"open", "show", "set", "history", "undo", "restore", "export"} {
			noteTestAction(t, a, c, `{"action":"`+verb+`","id":"group","revision":"r1","to":"pdf"}`, 403)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, c.request(http.MethodGet, "/?id=group", nil, a.outpostSecret))
		if w.Code != 403 || strings.Contains(w.Body.String(), "Member second version") || !strings.Contains(w.Body.String(), "Name or email") {
			t.Fatalf("unexpected blocked page: HTTP %d", w.Code)
		}
	}

	_, invite := noteTestAction(t, a, owner, `{"action":"share","id":"group"}`, 200)
	code := invite["invite"].(string)
	noteTestAction(t, a, bad, `{"action":"join","id":"group","invite":"`+code+`","handle":"Pretending"}`, 403)
	noteTestAction(t, a, guest, `{"action":"join","id":"group","invite":"`+code+`"}`, 403)
	w, _ := noteTestAction(t, a, guest, `{"action":"join","id":"group","invite":"`+code+`","handle":"Ana"}`, 200)
	for _, c := range w.Result().Cookies() {
		if c.Name == noteParticipantCookie {
			guest.cookie = c
		}
	}
	if guest.cookie == nil {
		t.Fatal("guest did not receive a participant cookie")
	}
	noteTestAction(t, a, outsider, `{"action":"join","id":"group","invite":"`+code+`","handle":"Other"}`, 403)
	foreign := guest
	foreign.address = "192.0.2.5"
	noteTestAction(t, a, foreign, `{"action":"set","id":"group","body":"Foreign cookie"}`, 403)
	noteTestAction(t, a, guest, `{"action":"set","id":"group","base_rev":"r2","body":"Ana third version"}`, 200)
	_, invite = noteTestAction(t, a, owner, `{"action":"share","id":"group"}`, 200)
	noteTestAction(t, a, outsider, `{"action":"join","id":"group","invite":"`+invite["invite"].(string)+`","handle":"Account guest"}`, 200)
	noteTestAction(t, a, outsider, `{"action":"set","id":"group","base_rev":"r3","body":"Account fourth version"}`, 200)
	_, invite = noteTestAction(t, a, owner, `{"action":"share","id":"group"}`, 200)
	noteTestAction(t, a, legacyOutsider, `{"action":"join","id":"group","invite":"`+invite["invite"].(string)+`","handle":"Legacy alias guest"}`, 200)
	noteTestAction(t, a, legacyOutsider, `{"action":"set","id":"group","base_rev":"r4","body":"Legacy alias fifth version"}`, 200)

	rec, _ := noteTestAction(t, a, guest, `{"action":"history","id":"group"}`, 200)
	var history kit.RevisionHistory
	if err := json.Unmarshal(rec.Body.Bytes(), &history); err != nil || history.Active != "r5" || history.Previous != "r4" || len(history.Versions) != 5 {
		t.Fatalf("history = %+v, %v", history, err)
	}
	for i, want := range []struct{ author, kind string }{
		{"guest-fedcba98", "account"}, {"guest-01234567", "account"}, {"Ana", "anonymous"}, {"member@example.test", "member"}, {"owner@example.test", "member"},
	} {
		if got := history.Versions[i]; got.Author != want.author || got.Kind != want.kind {
			t.Errorf("revision %s: %q (%s), want %q (%s)", got.Revision, got.Author, got.Kind, want.author, want.kind)
		}
	}

	noteTestAction(t, a, guest, `{"action":"undo","id":"group"}`, 200)
	_, view := noteTestAction(t, a, member, `{"action":"show","id":"group"}`, 200)
	if view["body"] != "Account fourth version" || view["active"] != "r4" {
		t.Fatalf("undo view: %v", view)
	}
	noteTestAction(t, a, owner, `{"action":"restore","id":"group","revision":"r1"}`, 200)
	_, view = noteTestAction(t, a, guest, `{"action":"show","id":"group"}`, 200)
	if view["body"] != "Owner first version" || view["active"] != "r1" || view["title"] != "Group note" {
		t.Fatalf("restore view: %v", view)
	}
	_, exported := noteTestAction(t, a, member, `{"action":"export","id":"group","rev":"3","to":"pdf"}`, 200)
	pdf, err := os.ReadFile(exported["path"].(string))
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("exported PDF: %v", err)
	}
	text, _, err := ConvertContent(pdf, "note.pdf")
	if err != nil || !strings.Contains(text, "Ana third version") || strings.Contains(text, "Owner first version") {
		t.Fatalf("named revision export: %q (%v)", text, err)
	}
	_, view = noteTestAction(t, a, guest, `{"action":"show","id":"group"}`, 200)
	if view["active"] != "r1" {
		t.Fatalf("export changed the active revision: %v", view)
	}
}

func TestNoteRevisionCLIAndLegacyBody(t *testing.T) {
	url, dir := startServer(t, "")
	legacy := &note{
		Version: 1, ID: "legacy", Title: "Legacy note", Body: "Original legacy body",
		Status: kit.StatusFilled, Owner: kit.NewToken(),
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := kit.AtomicWrite(notePath(dir, legacy.ID), raw); err != nil {
		t.Fatal(err)
	}

	call := func(want int, args ...string) map[string]any {
		t.Helper()
		var out bytes.Buffer
		argv := append([]string{"--addr", strings.TrimPrefix(url, "http://"), "--data", dir, "--json"}, args...)
		if code := runCLI(argv, &out); code != want {
			t.Fatalf("notepad %v: exit %d, want %d: %s", args, code, want, out.String())
		}
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("notepad %v: %v: %s", args, err, out.String())
		}
		return result
	}
	call(0, "set", "legacy", "--body", "New body")
	call(0, "set", "legacy", "--title", "Updated title")
	if got := call(0, "history", "legacy"); got["active"] != "r2" || len(got["versions"].([]any)) != 2 {
		t.Fatalf("history after first body write and metadata update: %v", got)
	}
	if got := call(1, "set", "legacy", "--base-rev", "r1", "--body", "Stale CLI body"); got["active"] != "r2" || got["base_rev"] != "r1" {
		t.Fatalf("CLI conflict: %v", got)
	}
	call(0, "undo", "legacy")
	if got := call(0, "show", "legacy"); got["body"] != "Original legacy body" || got["title"] != "Updated title" {
		t.Fatalf("legacy undo: %v", got)
	}
	call(0, "restore", "legacy", "r2")
	out := call(0, "export", "legacy", "--rev", "1", "--to", "txt")
	if raw, err := os.ReadFile(out["path"].(string)); err != nil || !strings.Contains(string(raw), "Original legacy body") {
		t.Fatalf("legacy export: %q, %v", raw, err)
	}
}

func TestNoteBareMountOnlyAllowsCreation(t *testing.T) {
	a := &app{dir: t.TempDir(), notes: map[string]*note{}}
	serve := func(input, header, value string) int {
		req := (noteTestCaller{}).request(http.MethodPost, "/api/action", []byte(input), "")
		req.Header.Set("X-Forwarded-Prefix", "/desk/notepad")
		if header != "" {
			req.Header.Set(header, value)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, req)
		return w.Code
	}
	if got := serve(`{"action":"add","id":"local"}`, "", ""); got != http.StatusOK {
		t.Fatalf("local add with a bare mount hint: HTTP %d", got)
	}
	if got := serve(`{"action":"show","id":"local"}`, "", ""); got != http.StatusForbidden {
		t.Fatalf("show with a bare mount hint: HTTP %d", got)
	}
	for _, extra := range []struct{ name, value string }{
		{"X-Forwarded-For", "192.0.2.15"},
		{"Forwarded", "for=192.0.2.15"},
		{"X-Forwarded-Host", "proxy.example.test"},
		{"X-Forwarded-Proto", "https"},
		{"Remote-User", "visitor@example.test"},
	} {
		if got := serve(`{"action":"add","id":"remote"}`, extra.name, extra.value); got != http.StatusForbidden {
			t.Errorf("add via %s: HTTP %d", extra.name, got)
		}
	}
}
