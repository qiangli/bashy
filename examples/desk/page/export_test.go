package main

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var exportSecrets = []string{"192.0.2.7", "fp-7f3a", "cookie-9b1c", "facilitator-secret-key"}

func exportPage(t *testing.T, a *app) string {
	t.Helper()
	srv := fakeSibling(t)
	t.Setenv("VIEWER_ADDR", srv.URL)
	t.Setenv("NOTEPAD_ADDR", srv.URL)
	body := "# Session record\n\nIntro.\n\n" +
		"```block book:sprint-537/p1\n```\n\n" +
		"```block note:note-1\n```\n\n" +
		"```block embed:/whiteboard/?room=r1 id=board title=Sketch\n```\n\n" +
		"```block timer:break duration=90s\n```\n\n" +
		"```block here\n```\n"
	key := addPageBody(t, a, "p", body)
	a.mu.Lock()
	next := a.items["p"].clone()
	next.Participants = participantMap{"ada@192.0.2.7": {Handle: "ada", Address: "192.0.2.7", Fingerprint: "fp-7f3a",
		Cookie: "cookie-9b1c", Auth: "anonymous", JoinedAt: "2026-11-29T10:00:00Z"}}
	next.BlockFaces = map[string]storedFaces{"board": {faceSettings: faceSettings{OrganizerURL: "/brainstorm/?s=1#facilitator=facilitator-secret-key"}, Kind: "embed", Ref: "/whiteboard/?room=r1"}}
	if err := a.saveLocked(&next, ""); err != nil {
		t.Fatal(err)
	}
	a.mu.Unlock()
	_ = srv
	return key
}

func assertNoExportSecrets(t *testing.T, name, body, owner string) {
	t.Helper()
	for _, s := range append(append([]string{}, exportSecrets...), owner) {
		if s != "" && strings.Contains(body, s) {
			t.Errorf("%s carries secret %q", name, s)
		}
	}
}

func TestExportMarkdownSessionRecord(t *testing.T) {
	_, a := startServer(t, "")
	key := exportPage(t, a)
	owner := map[string]string{"X-Facilitator-Key": key}
	w := do(a, "GET", "/api/export?id=p&to=md", "", "127.0.0.1:5000", owner)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/markdown") {
		t.Fatalf("md export: %d %s", w.Code, w.Body)
	}
	md := w.Body.String()
	it := a.items["p"]
	for _, want := range []string{"# Session record", "`p`", it.Status, it.Audience, "revision: " + strconv.Itoa(it.Revision),
		"# Hello", "Book *content*.", "Note **body** here.", "](/whiteboard/?room=r1)",
		"break: 1:30 (ready)", "## Participants", "- ada (anonymous)"} {
		if !strings.Contains(md, want) {
			t.Errorf("md record lacks %q:\n%s", want, md)
		}
	}
	// Body order: book, note, embed, timer, here.
	idx := []int{strings.Index(md, "# Hello"), strings.Index(md, "Note **body**"), strings.Index(md, "/whiteboard/?room=r1"),
		strings.Index(md, "break: 1:30"), strings.Index(md, "## Participants")}
	for i := 1; i < len(idx); i++ {
		if idx[i] < idx[i-1] {
			t.Errorf("md record out of body order: %v\n%s", idx, md)
		}
	}
	if strings.Contains(md, "```block") {
		t.Errorf("md record carries raw block fences:\n%s", md)
	}
	assertNoExportSecrets(t, "md export", md, key)
	if w := do(a, "GET", "/api/export?id=p&to=markdown", "", "127.0.0.1:5000", owner); w.Code != 200 || w.Body.String() != md {
		t.Errorf("markdown alias: %d", w.Code)
	}
	if w := do(a, "GET", "/api/export?id=p&to=md", "", "127.0.0.1:5000", nil); w.Code != http.StatusForbidden {
		t.Errorf("md export without owner: want 403, got %d", w.Code)
	}
}

func TestExportMarkdownBookFallback(t *testing.T) {
	_, a := startServer(t, "")
	srv := fakeSibling(t)
	t.Setenv("VIEWER_ADDR", srv.URL)
	key := addPageBody(t, a, "p", "# C\n\n```block book:missing/p1\n```\n")
	md := do(a, "GET", "/api/export?id=p&to=md", "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key}).Body.String()
	if !strings.Contains(md, "not available") {
		t.Errorf("unreachable/missing book should degrade in md:\n%s", md)
	}
}

func TestExportHTMLSelfContained(t *testing.T) {
	_, a := startServer(t, "")
	key := exportPage(t, a)
	owner := map[string]string{"X-Facilitator-Key": key}
	w := do(a, "GET", "/api/export?id=p&to=html", "", "127.0.0.1:5000", owner)
	if w.Code != 200 {
		t.Fatalf("html export: %d", w.Code)
	}
	page := w.Body.String()
	for _, want := range []string{"<style>", "Hello", "<strong>body</strong>", `data-block="board"`, "/whiteboard/?room=r1", "Participants", "ada"} {
		if !strings.Contains(page, want) {
			t.Errorf("html record lacks %q", want)
		}
	}
	for _, bad := range []string{"<link", "<script src", "src=\"http", "href=\"http", "data-native-control", "data-timer-action",
		`<button type="submit"`, "<iframe"} {
		if strings.Contains(page, bad) {
			t.Errorf("html record carries interactive/external %q", bad)
		}
	}
	assertNoExportSecrets(t, "html export", page, key)
	if w := do(a, "GET", "/api/export?id=p&to=html", "", "127.0.0.1:5000", nil); w.Code != http.StatusForbidden {
		t.Errorf("html export without owner: want 403, got %d", w.Code)
	}
}

// fakeChrome installs a CHROME_PATH shell script that records its argv,
// captures the HTML it was asked to print, and writes a minimal PDF.
func fakeChrome(t *testing.T) (argvLog, htmlCap string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake chrome shell script is POSIX only")
	}
	dir := t.TempDir()
	argvLog = filepath.Join(dir, "argv.txt")
	htmlCap = filepath.Join(dir, "export.html")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > '" + argvLog + "'\n" +
		"for a in \"$@\"; do\n" +
		"  case \"$a\" in --print-to-pdf=*) printf '%%PDF-1.4\\n' > \"${a#--print-to-pdf=}\" ;; file://*) cp \"${a#file://}\" '" + htmlCap + "' ;; esac\n" +
		"done\n"
	fake := filepath.Join(dir, "fake-chrome.sh")
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHROME_PATH", fake)
	return argvLog, htmlCap
}

func checkFakeChrome(t *testing.T, argvLog, htmlCap, owner string) {
	t.Helper()
	raw, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for _, want := range []string{"--headless=new", "--disable-gpu", "--no-sandbox", "--no-pdf-header-footer"} {
		if !slices.Contains(got, want) {
			t.Errorf("chrome argv missing %q: %v", want, got)
		}
	}
	ok := false
	for _, a := range got {
		if strings.HasPrefix(a, "--user-data-dir=") && strings.Contains(a, "page-pdf-") && strings.HasSuffix(a, "/profile") {
			ok = true
		}
		if strings.HasPrefix(a, "--print-to-pdf=") {
			ok = ok && true
		}
	}
	if !ok {
		t.Errorf("chrome argv lacks throwaway profile dir: %v", got)
	}
	htmlBytes, err := os.ReadFile(htmlCap)
	if err != nil {
		t.Fatal(err)
	}
	assertNoExportSecrets(t, "pdf html input", string(htmlBytes), owner)
	if !strings.Contains(string(htmlBytes), "Hello") {
		t.Errorf("pdf html input lacks book content")
	}
}

func TestExportMarkdownAnonymousPollHidesHandles(t *testing.T) {
	url, a, key := classPageWithForm(t)
	if code, out := cli(url, a)("block", "add", "poll", "class", "--options", "a,b", "--anonymous", "--block", "secret-ballot"); code != 0 {
		t.Fatalf("block add: %s", out)
	}
	ana := &door{t: t, a: a, peer: "192.0.2.10:4000"}
	ben := &door{t: t, a: a, peer: "192.0.2.11:4000"}
	ana.join("Ana")
	ben.join("Ben")
	ana.answer("secret-ballot", map[string]any{"vote": "b"})
	ben.answer("secret-ballot", map[string]any{"vote": "b"})
	md := do(a, "GET", "/api/export?id=class&to=md", "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key}).Body.String()
	section, _, _ := strings.Cut(md, "## Participants")
	if !strings.Contains(section, "| b | 2 |") || strings.Contains(section, "Ana") || strings.Contains(section, "Ben") {
		t.Errorf("anonymous export tally wrong:\n%s", section)
	}
	// Participants section still lists handles (the poll itself is anonymous).
	if !strings.Contains(md, "## Participants") {
		t.Errorf("export lacks participants section:\n%s", md)
	}
}

func TestExportPDFViaFakeChrome(t *testing.T) {
	_, a := startServer(t, "")
	key := exportPage(t, a)
	argvLog, htmlCap := fakeChrome(t)
	owner := map[string]string{"X-Facilitator-Key": key}
	if w := do(a, "GET", "/api/export?id=p&to=pdf", "", "127.0.0.1:5000", nil); w.Code != http.StatusForbidden {
		t.Fatalf("pdf export without owner: want 403, got %d", w.Code)
	}
	w := do(a, "GET", "/api/export?id=p&to=pdf", "", "127.0.0.1:5000", owner)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(w.Body.String(), "%PDF-1.4") {
		t.Fatalf("pdf export: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String()[:20])
	}
	checkFakeChrome(t, argvLog, htmlCap, key)
}

func TestExportPDFCLIViaFakeChrome(t *testing.T) {
	url, a := startServer(t, "")
	key := exportPage(t, a)
	argvLog, htmlCap := fakeChrome(t)
	out := filepath.Join(t.TempDir(), "record.pdf")
	code, stdout := cli(url, a)("export", "p", "--to", "pdf", "-o", out)
	if code != 0 {
		t.Fatalf("cli pdf export: %d %s (key %s)", code, stdout, key)
	}
	data, err := os.ReadFile(out)
	if err != nil || !strings.HasPrefix(string(data), "%PDF-1.4") {
		t.Fatalf("cli pdf file: %v %q", err, data)
	}
	checkFakeChrome(t, argvLog, htmlCap, a.items["p"].Owner)
	args := chromePDFArgs("/tmp/page-pdf-1/profile", "/tmp/out.pdf", "/tmp/page-pdf-1/export.html")
	for _, want := range []string{"--headless=new", "--disable-gpu", "--no-sandbox", "--no-pdf-header-footer", "--user-data-dir=/tmp/page-pdf-1/profile", "--print-to-pdf=/tmp/out.pdf", "file:///tmp/page-pdf-1/export.html"} {
		if !slices.Contains(args, want) {
			t.Errorf("chromePDFArgs missing %q", want)
		}
	}
}
