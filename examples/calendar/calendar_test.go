package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"desk-kit/contracttest"
)

func startTestServer(t *testing.T, breakM string) (string, string, *app) {
	t.Helper()
	old := breakMode
	breakMode = breakM
	t.Cleanup(func() { breakMode = old })

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "items"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{
		dir:    dir,
		events: map[string]*event{},
	}
	srv := httptest.NewUnstartedServer(a)
	a.baseURL = "http://" + srv.Listener.Addr().String()
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL, dir, a
}

func deskCLICallback(srvURL, dataDir string) func(args ...string) (int, string) {
	addr := strings.TrimPrefix(srvURL, "http://")
	return func(args ...string) (int, string) {
		var out bytes.Buffer
		full := append([]string{"-addr", addr, "-data", dataDir}, args...)
		code := runCLIWithIO(full, strings.NewReader(""), &out)
		return code, out.String()
	}
}

func runTestCLI(t *testing.T, srvURL, dataDir string, stdin string, args ...string) (int, string) {
	t.Helper()
	addr := strings.TrimPrefix(srvURL, "http://")
	var out bytes.Buffer
	full := append([]string{"-addr", addr, "-data", dataDir}, args...)
	code := runCLIWithIO(full, strings.NewReader(stdin), &out)
	return code, out.String()
}

// 1. Desk Contract Test (green on clean server + CLI, red on deliberate breaks)
func TestDeskContract(t *testing.T) {
	srvURL, dataDir, _ := startTestServer(t, "")
	contracttest.Run(t, contracttest.Config{
		BaseURL: srvURL,
		Tool:    "calendar",
		CLI:     deskCLICallback(srvURL, dataDir),
	})
}

func TestContractRedOnBreak(t *testing.T) {
	for _, b := range []string{"open-emits-content", "no-csp", "allow-secret", "hex-color", "font-family", "no-shell", "no-schema-version", "no-render"} {
		srvURL, _, _ := startTestServer(t, b)
		fails := contracttest.Check(contracttest.Config{BaseURL: srvURL, Tool: "calendar"})
		if len(fails) == 0 {
			t.Errorf("break %q: contract test stayed green, want red", b)
		}
	}
}

// 2. Explicit IANA Timezones, DST Edges, Invalid TZ Refusal, and Never Deriving from Created/Updated
func TestTimezoneValidationAndDST(t *testing.T) {
	srvURL, dataDir, _ := startTestServer(t, "")

	// Invalid IANA timezones are refused with exit 2 (CLI) / HTTP 400.
	for _, badTZ := range []string{"Mars/Olympus", "Not/AZone", "local", "Local", "../UTC", ""} {
		code, out := runTestCLI(t, srvURL, dataDir, "", "--json", "add", "--title", "Bad TZ", "--tz", badTZ)
		if code != 2 {
			t.Fatalf("bad tz %q: want exit 2, got %d (%s)", badTZ, code, out)
		}
	}

	// Malformed timestamps and end < start are refused with exit 2 / HTTP 400.
	if code, out := runTestCLI(t, srvURL, dataDir, "", "--json", "add", "--title", "Bad Start", "--start", "not-a-date", "--tz", "UTC"); code != 2 {
		t.Fatalf("bad start: want exit 2, got %d (%s)", code, out)
	}
	if code, out := runTestCLI(t, srvURL, dataDir, "", "--json", "add",
		"--title", "Backwards",
		"--start", "2026-11-21T12:00:00Z",
		"--end", "2026-11-21T11:00:00Z",
		"--tz", "UTC"); code != 2 {
		t.Fatalf("end < start: want exit 2, got %d (%s)", code, out)
	}

	// Omitted start/end/tz uses deterministic 2026-01-01T09:00:00Z (NEVER derived from created/updated).
	code, out := runTestCLI(t, srvURL, dataDir, "", "--json", "add", "det-default", "--title", "Default Window")
	if code != 0 {
		t.Fatalf("add default: exit %d: %s", code, out)
	}
	code, showOut := runTestCLI(t, srvURL, dataDir, "", "--json", "show", "det-default")
	if code != 0 {
		t.Fatalf("show det-default: exit %d: %s", code, showOut)
	}
	var det map[string]any
	_ = json.Unmarshal([]byte(showOut), &det)
	if det["start"] != "2026-01-01T09:00:00Z" || det["end"] != "2026-01-01T10:00:00Z" || det["tz"] != "UTC" {
		t.Fatalf("unexpected default window: start=%v end=%v tz=%v", det["start"], det["end"], det["tz"])
	}
	if det["start"] == det["created"] {
		t.Fatalf("event start must never be derived from created timestamp")
	}

	// US Spring-Forward DST boundary: 2026-03-08 in America/Los_Angeles jumps 02:00 PST -> 03:00 PDT.
	// 09:30Z is 01:30-08:00 (PST); 10:30Z (1h later) is 03:30-07:00 (PDT).
	code, out = runTestCLI(t, srvURL, dataDir, "", "--json", "add", "dst-spring",
		"--title", "DST Transition Check",
		"--start", "2026-03-08T09:30:00Z",
		"--end", "2026-03-08T10:30:00Z",
		"--tz", "UTC",
		"--audience", "alice",
		"--ref", "sprint:616",
		"--ref", "kb:enable-an-app-for-consumer-shaping")
	if code != 0 {
		t.Fatalf("add dst-spring: exit %d: %s", code, out)
	}

	// View in America/Los_Angeles: crosses PST (-08:00) into PDT (-07:00)
	code, listLA := runTestCLI(t, srvURL, dataDir, "", "--json", "list",
		"--tz", "America/Los_Angeles", "--from", "2026-03-08", "--to", "2026-03-08", "--audience", "alice")
	if code != 0 {
		t.Fatalf("list LA: exit %d: %s", code, listLA)
	}
	var laObj struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal([]byte(listLA), &laObj)
	if len(laObj.Items) != 1 {
		t.Fatalf("want 1 item on 2026-03-08 in LA, got %d (%s)", len(laObj.Items), listLA)
	}
	if laObj.Items[0]["display_start"] != "2026-03-08T01:30:00-08:00" || laObj.Items[0]["display_end"] != "2026-03-08T03:30:00-07:00" {
		t.Errorf("DST conversion mismatch: got %v -> %v", laObj.Items[0]["display_start"], laObj.Items[0]["display_end"])
	}

	// View late-night LA event in Asia/Tokyo: crosses date line to next calendar date.
	code, out = runTestCLI(t, srvURL, dataDir, "", "--json", "add", "dateline",
		"--title", "Late Pacific Sync",
		"--start", "2026-06-15T23:00:00-07:00",
		"--end", "2026-06-16T00:00:00-07:00",
		"--tz", "America/Los_Angeles")
	if code != 0 {
		t.Fatalf("add dateline: exit %d: %s", code, out)
	}
	code, showTokyo := runTestCLI(t, srvURL, dataDir, "", "--json", "show", "dateline", "--tz", "Asia/Tokyo")
	if code != 0 {
		t.Fatalf("show Tokyo: exit %d: %s", code, showTokyo)
	}
	var tokyo map[string]any
	_ = json.Unmarshal([]byte(showTokyo), &tokyo)
	if tokyo["date"] != "2026-06-16" || tokyo["display_start"] != "2026-06-16T15:00:00+09:00" {
		t.Errorf("Tokyo dateline mismatch: date=%v display_start=%v", tokyo["date"], tokyo["display_start"])
	}

	// Invalid --tz on list/show returns exit 2.
	if code, _ := runTestCLI(t, srvURL, dataDir, "", "--json", "list", "--tz", "Bogus/Zone"); code != 2 {
		t.Errorf("list invalid tz: want exit 2, got %d", code)
	}
}

// 3. Cancel vs Close Lifecycle + Owner / Single-Use Invite / Member / Revoke 404 Parity
func TestAudienceInviteRevoke404AndCancelLifecycle(t *testing.T) {
	srvURL, dataDir, _ := startTestServer(t, "")

	code, createdRaw := runTestCLI(t, srvURL, dataDir, "", "--json", "add", "team-sync",
		"--title", "Team Sync",
		"--start", "2026-11-21T18:00:00Z",
		"--end", "2026-11-21T18:30:00Z",
		"--tz", "UTC",
		"--body", "Confidential roadmap")
	if code != 0 {
		t.Fatalf("add: exit %d: %s", code, createdRaw)
	}
	var created map[string]any
	_ = json.Unmarshal([]byte(createdRaw), &created)
	ownerKey := created["ownerKey"].(string)

	// Non-member HTTP client receives HTTP 404 (NEVER 403) on state, open, show, set, cancel, close, share, export.
	stranger := &http.Client{Timeout: 5 * time.Second}
	for _, probe := range []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/state?id=team-sync", ""},
		{"GET", "/api/export?id=team-sync&to=ics", ""},
		{"POST", "/api/action", `{"action":"open","id":"team-sync"}`},
		{"POST", "/api/action", `{"action":"show","id":"team-sync"}`},
		{"POST", "/api/action", `{"action":"set","id":"team-sync","title":"Hijack"}`},
		{"POST", "/api/action", `{"action":"cancel","id":"team-sync"}`},
		{"POST", "/api/action", `{"action":"close","id":"team-sync"}`},
		{"POST", "/api/action", `{"action":"share","id":"team-sync"}`},
		{"POST", "/api/action", `{"action":"export","id":"team-sync","to":"ics"}`},
	} {
		var req *http.Request
		if probe.method == "GET" {
			req, _ = http.NewRequest("GET", srvURL+probe.path, nil)
		} else {
			req, _ = http.NewRequest("POST", srvURL+probe.path, strings.NewReader(probe.body))
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := stranger.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("non-member %s %s %s: want HTTP 404, got %d", probe.method, probe.path, probe.body, res.StatusCode)
		}
	}

	// Non-member CLI (empty dataDir with no saved owner key) surfaces denied failure (non-zero exit).
	emptyDir := t.TempDir()
	for _, verb := range []string{"open", "show", "cancel", "close", "share"} {
		c, out := runTestCLI(t, srvURL, emptyDir, "", "--json", verb, "team-sync")
		if c == 0 {
			t.Errorf("non-member CLI %s succeeded unexpectedly: %s", verb, out)
		}
	}
	if c, out := runTestCLI(t, srvURL, emptyDir, "", "--json", "export", "team-sync", "--to", "ics"); c == 0 {
		t.Errorf("non-member CLI export succeeded unexpectedly: %s", out)
	}

	// Owner mints a single-use invite via CLI `share`, prints join URL via `join`.
	code, shareRaw := runTestCLI(t, srvURL, dataDir, "", "--json", "share", "team-sync")
	if code != 0 {
		t.Fatalf("share: exit %d: %s", code, shareRaw)
	}
	var shared map[string]any
	_ = json.Unmarshal([]byte(shareRaw), &shared)
	invite := shared["invite"].(string)

	code, joinOut := runTestCLI(t, srvURL, emptyDir, "", "join", "team-sync", invite)
	if code != 0 || !strings.Contains(joinOut, "#invite="+invite) {
		t.Fatalf("join URL: exit %d: %s", code, joinOut)
	}

	// Member browser (with CookieJar) redeems single-use invite via POST /api/action join.
	jar, _ := cookiejar.New(nil)
	memberHTTP := &http.Client{Timeout: 5 * time.Second, Jar: jar}
	joinReq, _ := http.NewRequest("POST", srvURL+"/api/action",
		strings.NewReader(`{"action":"join","id":"team-sync","invite":"`+invite+`"}`))
	joinReq.Header.Set("Content-Type", "application/json")
	joinRes, err := memberHTTP.Do(joinReq)
	if err != nil {
		t.Fatal(err)
	}
	var joined map[string]any
	_ = json.NewDecoder(joinRes.Body).Decode(&joined)
	joinRes.Body.Close()
	if joinRes.StatusCode != http.StatusOK {
		t.Fatalf("member join: want 200, got %d (%v)", joinRes.StatusCode, joined)
	}
	memberTokenStr, _ := joined["member"].(string)
	if memberTokenStr == "" {
		t.Fatalf("join must return member token: %v", joined)
	}

	// Reusing the single-use invite fails with 404.
	replayRes, err := stranger.Post(srvURL+"/api/action", "application/json",
		strings.NewReader(`{"action":"join","id":"team-sync","invite":"`+invite+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	replayRes.Body.Close()
	if replayRes.StatusCode != http.StatusNotFound {
		t.Errorf("replayed single-use invite: want 404, got %d", replayRes.StatusCode)
	}

	// Joined member can read state, show, and export (both via cookie and CLI --member).
	showRes, err := memberHTTP.Post(srvURL+"/api/action", "application/json",
		strings.NewReader(`{"action":"show","id":"team-sync"}`))
	if err != nil {
		t.Fatal(err)
	}
	var memberShown map[string]any
	_ = json.NewDecoder(showRes.Body).Decode(&memberShown)
	showRes.Body.Close()
	if showRes.StatusCode != http.StatusOK || memberShown["body"] != "Confidential roadmap" {
		t.Fatalf("member show failed: status=%d body=%v", showRes.StatusCode, memberShown)
	}
	if c, out := runTestCLI(t, srvURL, emptyDir, "", "--json", "--member", memberTokenStr, "show", "team-sync"); c != 0 {
		t.Fatalf("CLI --member show failed: exit %d: %s", c, out)
	}

	// Owner revokes member via `calendar set ID --revoke MEMBER`; revoked member immediately gets 404!
	code, revOut := runTestCLI(t, srvURL, emptyDir, "", "--json", "--key", ownerKey, "set", "team-sync", "--revoke", memberTokenStr)
	if code != 0 {
		t.Fatalf("revoke: exit %d: %s", code, revOut)
	}
	afterRevoke, err := memberHTTP.Post(srvURL+"/api/action", "application/json",
		strings.NewReader(`{"action":"show","id":"team-sync"}`))
	if err != nil {
		t.Fatal(err)
	}
	afterRevoke.Body.Close()
	if afterRevoke.StatusCode != http.StatusNotFound {
		t.Errorf("revoked member HTTP show: want 404, got %d", afterRevoke.StatusCode)
	}
	if c, _ := runTestCLI(t, srvURL, emptyDir, "", "--json", "--member", memberTokenStr, "show", "team-sync"); c == 0 {
		t.Errorf("revoked member CLI show must fail non-zero")
	}

	// `cancel` transitions status to "cancelled" and keeps the item readable.
	code, cancelOut := runTestCLI(t, srvURL, dataDir, "", "--json", "cancel", "team-sync")
	if code != 0 {
		t.Fatalf("cancel: exit %d: %s", code, cancelOut)
	}
	var cancelled map[string]any
	_ = json.Unmarshal([]byte(cancelOut), &cancelled)
	if cancelled["status"] != "cancelled" {
		t.Errorf("cancel: want status cancelled, got %v", cancelled["status"])
	}
	code, showAfterCancel := runTestCLI(t, srvURL, dataDir, "", "--json", "show", "team-sync")
	if code != 0 || !strings.Contains(showAfterCancel, `"status":"cancelled"`) {
		t.Errorf("cancelled event must remain readable: exit=%d out=%s", code, showAfterCancel)
	}

	// `close` transitions to "closed" and subsequent `set` or `cancel` fails (HTTP 409 / non-zero exit).
	if code, out := runTestCLI(t, srvURL, dataDir, "", "--json", "close", "team-sync"); code != 0 {
		t.Fatalf("close: exit %d: %s", code, out)
	}
	if code, _ := runTestCLI(t, srvURL, dataDir, "", "--json", "set", "team-sync", "--title", "After Close"); code == 0 {
		t.Errorf("set after close must fail")
	}
}

// 4. ICS Export/Import, --dry-run Zero Mutation, VALUE=DATE, Folding, and source.uid Dedupe
func TestICSImportExportDryRunAndDedupe(t *testing.T) {
	srvURL, dataDir, _ := startTestServer(t, "")

	icsSample := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Test//Calendar//EN",
		"BEGIN:VEVENT",
		"UID:ext-ics-uid-42@example.org",
		"DTSTART;TZID=America/New_York:20261124T140000",
		"DTEND;TZID=America/New_York:20261124T150000",
		"SUMMARY:Architecture Review\\, Phase 1",
		"DESCRIPTION:Line one\\nLine two with folded ",
		" continuation text.",
		"LOCATION:Building 7\\; Room 2",
		"URL:https://example.org/arch",
		"URL:kb:enable-an-app-for-consumer-shaping",
		"STATUS:CONFIRMED",
		"END:VEVENT",
		"BEGIN:VEVENT",
		"UID:ext-allday-99@example.org",
		"DTSTART;VALUE=DATE:20261126",
		"DTEND;VALUE=DATE:20261127",
		"SUMMARY:Company Holiday",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n")

	icsPath := filepath.Join(t.TempDir(), "feed.ics")
	if err := os.WriteFile(icsPath, []byte(icsSample), 0644); err != nil {
		t.Fatal(err)
	}

	// --dry-run import validates and previews with ZERO state mutation.
	code, dryOut := runTestCLI(t, srvURL, dataDir, "", "--dry-run", "--json", "import", "-f", icsPath)
	if code != 0 {
		t.Fatalf("dry-run import: exit %d: %s", code, dryOut)
	}
	var dryObj map[string]any
	_ = json.Unmarshal([]byte(dryOut), &dryObj)
	if dryObj["dry_run"] != true || dryObj["created"] != float64(2) {
		t.Fatalf("dry-run import unexpected preview: %s", dryOut)
	}
	entries, _ := os.ReadDir(filepath.Join(dataDir, "items"))
	if len(entries) != 0 {
		t.Fatalf("--dry-run import mutated items dir: %d files", len(entries))
	}

	// Real import creates 2 events with source.provider="ics".
	code, impOut := runTestCLI(t, srvURL, dataDir, "", "--json", "import", "-f", icsPath)
	if code != 0 {
		t.Fatalf("import: exit %d: %s", code, impOut)
	}
	var imp1 map[string]any
	_ = json.Unmarshal([]byte(impOut), &imp1)
	if imp1["created"] != float64(2) || imp1["updated"] != float64(0) {
		t.Fatalf("first import counts wrong: %s", impOut)
	}

	// Re-importing updated ICS with the SAME UID updates in-place instead of duplicating.
	updatedICS := strings.Replace(icsSample, "Architecture Review\\, Phase 1", "Architecture Review\\, Phase 2", 1)
	code, impOut2 := runTestCLI(t, srvURL, dataDir, updatedICS, "--json", "import", "-f", "-", "--from", "ics")
	if code != 0 {
		t.Fatalf("re-import: exit %d: %s", code, impOut2)
	}
	var imp2 map[string]any
	_ = json.Unmarshal([]byte(impOut2), &imp2)
	if imp2["created"] != float64(0) || imp2["updated"] != float64(2) {
		t.Fatalf("re-import must update existing events by source.uid, got: %s", impOut2)
	}
	entries, _ = os.ReadDir(filepath.Join(dataDir, "items"))
	if len(entries) != 2 {
		t.Fatalf("want 2 items after idempotent re-import, got %d", len(entries))
	}

	// Export --all --to ics with --dry-run writes no file; real export writes valid ICS.
	exportPath := filepath.Join(t.TempDir(), "exported.ics")
	if code, out := runTestCLI(t, srvURL, dataDir, "", "--dry-run", "--json", "export", "--all", "--to", "ics", "-o", exportPath); code != 0 {
		t.Fatalf("dry-run export: exit %d: %s", code, out)
	}
	if _, err := os.Stat(exportPath); err == nil {
		t.Fatalf("--dry-run export must not create output file")
	}

	if code, out := runTestCLI(t, srvURL, dataDir, "", "--json", "export", "--all", "--to", "ics", "-o", exportPath); code != 0 {
		t.Fatalf("export: exit %d: %s", code, out)
	}
	exportedBytes, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := ParseICS(exportedBytes)
	if err != nil || len(reparsed) != 2 {
		t.Fatalf("exported ICS failed to re-parse: err=%v len=%d\n%s", err, len(reparsed), string(exportedBytes))
	}
	foundPhase2 := false
	for _, sp := range reparsed {
		if sp.Source.UID == "ext-ics-uid-42@example.org" {
			foundPhase2 = true
			if sp.Title != "Architecture Review, Phase 2" || sp.TZ != "America/New_York" || len(sp.Refs) != 2 {
				t.Errorf("round-tripped VEVENT mismatch: %+v", sp)
			}
		}
	}
	if !foundPhase2 {
		t.Errorf("exported ICS missing ext-ics-uid-42@example.org")
	}

	// Recurrence (RRULE) in ICS is explicitly refused in MVP.
	rruleICS := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:r1\r\nDTSTART:20261124T100000Z\r\nRRULE:FREQ=WEEKLY\r\nSUMMARY:Weekly\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if code, _ := runTestCLI(t, srvURL, dataDir, rruleICS, "--json", "import", "-f", "-"); code != 2 {
		t.Errorf("RRULE import must be refused with exit 2, got %d", code)
	}
}

// 5. Offline-Fixture Google Import/Export via BASHY_GOOGLE_CMD + Missing CLI Error
func TestGoogleImportExportOfflineFixturesAndMissingCLI(t *testing.T) {
	srvURL, dataDir, _ := startTestServer(t, "")

	// Missing Google CLI returns actionable CLI_NOT_FOUND error with onboarding hint.
	t.Setenv("BASHY_GOOGLE_CMD", "/nonexistent/bashy-google-cli")
	code, missOut := runTestCLI(t, srvURL, dataDir, "", "--json", "import", "--from", "google")
	if code == 0 || !strings.Contains(missOut, "CLI_NOT_FOUND") || !strings.Contains(missOut, "auth-guide") {
		t.Fatalf("missing google CLI: want CLI_NOT_FOUND + auth-guide hint, got exit=%d out=%s", code, missOut)
	}
	code, missExp := runTestCLI(t, srvURL, dataDir, "", "--json", "export", "--all", "--to", "google")
	if code == 0 || !strings.Contains(missExp, "CLI_NOT_FOUND") {
		t.Fatalf("missing google CLI on export: want CLI_NOT_FOUND, got exit=%d out=%s", code, missExp)
	}

	// Fake offline BASHY_GOOGLE_CMD script logging every invocation and serving JSON fixtures.
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "google-calls.log")
	fixtureFile := filepath.Join(tmp, "calendar-list.json")
	fakeGoogle := filepath.Join(tmp, "fake-google.sh")

	initialFixture := `{
  "items": [
    {
      "id": "gcal_evt_101",
      "summary": "Cloud Sync",
      "description": "Imported from Google fixture",
      "location": "Meet",
      "status": "confirmed",
      "htmlLink": "https://calendar.google.com/calendar/event?eid=101",
      "start": {"dateTime": "2026-11-22T10:00:00-08:00", "timeZone": "America/Los_Angeles"},
      "end": {"dateTime": "2026-11-22T10:30:00-08:00", "timeZone": "America/Los_Angeles"}
    }
  ]
}`
	if err := os.WriteFile(fixtureFile, []byte(initialFixture), 0644); err != nil {
		t.Fatal(err)
	}

	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "` + logFile + `"
verb="${1:-}"
case "$verb" in
  calendar-list)
    cat "` + fixtureFile + `"
    ;;
  calendar-create)
    printf '{"id":"gcal_minted_777","status":"confirmed"}\n'
    ;;
  calendar-update)
    ev_id="${2:-}"
    printf '{"id":"%s","status":"confirmed"}\n' "$ev_id"
    ;;
  *)
    printf '{"error":{"code":"USAGE","message":"unknown verb %s"}}\n' "$verb" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(fakeGoogle, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_GOOGLE_CMD", fakeGoogle)

	// Import from Google fixture -> creates 1 event with source={provider:"google", uid:"gcal_evt_101"}.
	code, gImp1 := runTestCLI(t, srvURL, dataDir, "", "--json", "import", "--from", "google", "--calendar", "team@example.com")
	if code != 0 {
		t.Fatalf("google import: exit %d: %s", code, gImp1)
	}
	if !strings.Contains(gImp1, `"created":1`) {
		t.Fatalf("expected created=1 on first google import: %s", gImp1)
	}

	// Update Google fixture summary and re-import -> updates existing event by source.uid!
	updatedFixture := strings.Replace(initialFixture, "Cloud Sync", "Cloud Sync (Updated)", 1)
	_ = os.WriteFile(fixtureFile, []byte(updatedFixture), 0644)
	code, gImp2 := runTestCLI(t, srvURL, dataDir, "", "--json", "import", "--from", "google", "--calendar", "team@example.com")
	if code != 0 || !strings.Contains(gImp2, `"updated":1`) || !strings.Contains(gImp2, `"created":0`) {
		t.Fatalf("expected updated=1 created=0 on second google import: exit=%d %s", code, gImp2)
	}

	// Add a brand-new local event and export it to Google twice:
	// 1st export -> calls calendar-create and records returned id "gcal_minted_777" in source.uid
	// 2nd export -> calls calendar-update gcal_minted_777 instead of duplicating!
	if code, out := runTestCLI(t, srvURL, dataDir, "", "--json", "add", "local-to-gcal",
		"--title", "Local Planning",
		"--start", "2026-11-23T16:00:00Z",
		"--end", "2026-11-23T17:00:00Z",
		"--tz", "UTC"); code != 0 {
		t.Fatalf("add local-to-gcal: exit %d: %s", code, out)
	}

	// Dry-run export mutates neither Google nor local source.
	callsBefore, _ := os.ReadFile(logFile)
	code, dryG := runTestCLI(t, srvURL, dataDir, "", "--dry-run", "--json", "export", "local-to-gcal", "--to", "google")
	if code != 0 || !strings.Contains(dryG, `"calendar-create"`) {
		t.Fatalf("dry-run google export: exit %d: %s", code, dryG)
	}
	callsAfterDry, _ := os.ReadFile(logFile)
	if string(callsBefore) != string(callsAfterDry) {
		t.Fatalf("--dry-run google export invoked external command unexpectedly")
	}

	// 1st real export -> calendar-create
	code, exp1 := runTestCLI(t, srvURL, dataDir, "", "--json", "export", "local-to-gcal", "--to", "google", "--calendar", "team@example.com")
	if code != 0 || !strings.Contains(exp1, `"created":1`) || !strings.Contains(exp1, `"gcal_minted_777"`) {
		t.Fatalf("1st google export: exit %d: %s", code, exp1)
	}

	// Verify local event now stores source: {"provider":"google","uid":"gcal_minted_777"}
	_, showLocal := runTestCLI(t, srvURL, dataDir, "", "--json", "show", "local-to-gcal")
	if !strings.Contains(showLocal, `"provider":"google"`) || !strings.Contains(showLocal, `"uid":"gcal_minted_777"`) {
		t.Fatalf("local event did not persist google source.uid: %s", showLocal)
	}

	// 2nd real export -> calendar-update gcal_minted_777
	code, exp2 := runTestCLI(t, srvURL, dataDir, "", "--json", "export", "local-to-gcal", "--to", "google", "--calendar", "team@example.com")
	if code != 0 || !strings.Contains(exp2, `"updated":1`) || !strings.Contains(exp2, `"created":0`) {
		t.Fatalf("2nd google export must update rather than duplicate: exit=%d %s", code, exp2)
	}

	logBytes, _ := os.ReadFile(logFile)
	if !strings.Contains(string(logBytes), "calendar-update gcal_minted_777") {
		t.Errorf("expected calendar-update gcal_minted_777 in call log:\n%s", string(logBytes))
	}
}

// 6. Durable Git-Dir save / load Round-Trip (Never Invokes git)
func TestSaveAndLoadGitDirWithoutGit(t *testing.T) {
	srvURL1, dataDir1, _ := startTestServer(t, "")

	// Put a poison `git` executable first on PATH: if save or load ever runs `git`, it fails loudly.
	poisonBin := t.TempDir()
	poisonGit := filepath.Join(poisonBin, "git")
	poisonMarker := filepath.Join(poisonBin, "git-invoked")
	if err := os.WriteFile(poisonGit, []byte("#!/bin/sh\ntouch "+poisonMarker+"\nexit 99\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", poisonBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, id := range []string{"evt-alpha", "evt-beta"} {
		if code, out := runTestCLI(t, srvURL1, dataDir1, "", "--json", "add", id,
			"--title", "Title "+id,
			"--start", "2026-11-28T09:00:00-05:00",
			"--end", "2026-11-28T10:00:00-05:00",
			"--tz", "America/New_York",
			"--body", "Body for "+id,
			"--ref", "sprint:616",
			"--audience", "qiangli"); code != 0 {
			t.Fatalf("add %s: exit %d: %s", id, code, out)
		}
	}

	repoCalDir := filepath.Join(t.TempDir(), "docs", "calendar")

	// --dry-run save creates zero files.
	if code, out := runTestCLI(t, srvURL1, dataDir1, "", "--dry-run", "--json", "save", "--dir", repoCalDir); code != 0 {
		t.Fatalf("save --dry-run: exit %d: %s", code, out)
	}
	if _, err := os.Stat(repoCalDir); err == nil {
		t.Fatalf("save --dry-run must not create target directory")
	}

	// Real save writes events.json + <id>.ics per event, with no capability secrets leaked.
	code, saveOut := runTestCLI(t, srvURL1, dataDir1, "", "--json", "save", "--dir", repoCalDir)
	if code != 0 {
		t.Fatalf("save: exit %d: %s", code, saveOut)
	}
	eventsJSON, err := os.ReadFile(filepath.Join(repoCalDir, "events.json"))
	if err != nil {
		t.Fatalf("missing events.json: %v", err)
	}
	if strings.Contains(string(eventsJSON), `"owner"`) || strings.Contains(string(eventsJSON), `"invites"`) {
		t.Errorf("events.json must not leak capability tokens: %s", string(eventsJSON))
	}
	for _, id := range []string{"evt-alpha", "evt-beta"} {
		if _, err := os.Stat(filepath.Join(repoCalDir, id+".ics")); err != nil {
			t.Errorf("missing %s.ics: %v", id, err)
		}
	}

	// Load into a brand-new calendar server instance twice -> idempotent!
	srvURL2, dataDir2, _ := startTestServer(t, "")
	if code, out := runTestCLI(t, srvURL2, dataDir2, "", "--dry-run", "--json", "load", repoCalDir); code != 0 {
		t.Fatalf("load --dry-run: exit %d: %s", code, out)
	}
	if ents, _ := os.ReadDir(filepath.Join(dataDir2, "items")); len(ents) != 0 {
		t.Fatalf("load --dry-run mutated server items")
	}

	code, load1 := runTestCLI(t, srvURL2, dataDir2, "", "--json", "load", repoCalDir)
	if code != 0 || !strings.Contains(load1, `"created":2`) {
		t.Fatalf("1st load: exit %d: %s", code, load1)
	}
	code, load2 := runTestCLI(t, srvURL2, dataDir2, "", "--json", "load", repoCalDir)
	if code != 0 || !strings.Contains(load2, `"updated":2`) || !strings.Contains(load2, `"created":0`) {
		t.Fatalf("2nd load must be idempotent (updated=2, created=0): exit=%d %s", code, load2)
	}

	code, showAlpha := runTestCLI(t, srvURL2, dataDir2, "", "--json", "show", "evt-alpha")
	if code != 0 || !strings.Contains(showAlpha, "Body for evt-alpha") || !strings.Contains(showAlpha, "America/New_York") {
		t.Errorf("loaded event content mismatch: exit=%d %s", code, showAlpha)
	}

	if _, err := os.Stat(poisonMarker); err == nil {
		t.Fatalf("save/load invoked git, violating the never-invokes-git contract")
	}
}

// 7. Consumer Shaping: Runtime Discovery via kit.DiscoverAppRuntime + Shaping Hooks
func TestDiscoverAppRuntimeAndShapingHooks(t *testing.T) {
	srvURL, dataDir, srvApp := startTestServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")

	// Clear any explicit handoff env vars so client verbs must call DiscoverAppRuntime.
	t.Setenv("CALENDAR_ADDR", "")
	t.Setenv("CALENDAR_DATA", "")
	t.Setenv("BASHY_APP", "")
	t.Setenv("BASHY_APP_ADDR", "")
	t.Setenv("BASHY_APP_DATA", "")

	// Case A: Discovery fails -> exits non-zero naming CALENDAR_ADDR and CALENDAR_DATA (no silent fallback to 18099).
	failBashy := filepath.Join(t.TempDir(), "fake-bashy-fail")
	_ = os.WriteFile(failBashy, []byte("#!/bin/sh\necho 'app calendar is not running' >&2\nexit 1\n"), 0755)
	t.Setenv("BASHY_BIN", failBashy)

	var out bytes.Buffer
	code := runCLIWithIO([]string{"--json", "list"}, strings.NewReader(""), &out)
	if code == 0 {
		t.Fatalf("expected non-zero exit when runtime discovery fails, got 0: %s", out.String())
	}
	if !strings.Contains(out.String(), "CALENDAR_ADDR") || !strings.Contains(out.String(), "CALENDAR_DATA") {
		t.Errorf("failure message must name CALENDAR_ADDR and CALENDAR_DATA, got: %s", out.String())
	}

	// Case B: Discovery succeeds -> routes to console addr/data and prints URLs under console mount!
	okBashy := filepath.Join(t.TempDir(), "fake-bashy-ok")
	runtimeJSON, _ := json.Marshal(map[string]string{
		"addr": addr,
		"data": dataDir,
		"url":  "https://console.example.test/calendar/",
	})
	_ = os.WriteFile(okBashy, []byte("#!/bin/sh\nprintf '%s\\n' '"+string(runtimeJSON)+"'\n"), 0755)
	t.Setenv("BASHY_BIN", okBashy)

	out.Reset()
	code = runCLIWithIO([]string{"--json", "add", "console-evt", "--title", "Shaped Console Event"}, strings.NewReader(""), &out)
	if code != 0 {
		t.Fatalf("discovered add failed: exit %d: %s", code, out.String())
	}
	var created map[string]any
	_ = json.Unmarshal(out.Bytes(), &created)
	gotURL, _ := created["url"].(string)
	if !strings.HasPrefix(gotURL, "https://console.example.test/calendar/?id=console-evt") {
		t.Errorf("printed URL must use console mount from DiscoverAppRuntime, got %q", gotURL)
	}

	// Case C: Consumer shaping edit to templates/agenda.html on disk takes effect live without recompile.
	shapedTmplDir := filepath.Join(dataDir, "templates")
	if err := os.MkdirAll(shapedTmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	customMarker := "<!-- consumer-shaped-agenda-banner -->"
	if err := os.WriteFile(filepath.Join(shapedTmplDir, "agenda.html"), []byte(shippedAgendaHTML+"\n"+customMarker), 0644); err != nil {
		t.Fatal(err)
	}
	_ = srvApp
	res, err := http.Get(srvURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	var pageBuf bytes.Buffer
	_, _ = pageBuf.ReadFrom(res.Body)
	res.Body.Close()
	if !strings.Contains(pageBuf.String(), customMarker) {
		t.Errorf("live edit to templates/agenda.html was not served by GET /")
	}
}
