package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	kit "github.com/qiangli/bashy/examples/desk/kit"
)

const presencePageBody = "# Workshop\n\n" +
	"```block here id=attendees\n```\n\n" +
	"```block here id=roster\n```\n\n" +
	"```block hands id=queue\n```\n\n" +
	"```block hands id=backup\n```\n"

const presenceAgent = "test-presence"
const presencePeer = "127.0.0.1:5000"

func setupPresencePage(t *testing.T) (string, *app, string) {
	t.Helper()
	base, a := startServer(t, "")
	raw, err := json.Marshal(actionRequest{Action: "add", ID: "group", Body: presencePageBody,
		JoinCode: "group", Audience: audienceLAN, Faces: map[string]*faceSettings{"queue": {Project: true}}})
	if err != nil {
		t.Fatal(err)
	}
	w := do(a, http.MethodPost, "/api/action", string(raw), presencePeer, nil)
	var created struct{ OwnerKey string }
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &created) != nil || created.OwnerKey == "" {
		t.Fatalf("create page: %d %s", w.Code, w.Body)
	}
	return base, a, created.OwnerKey
}

func joinPresencePage(t *testing.T, a *app, peer, handle string) map[string]string {
	t.Helper()
	w := do(a, http.MethodPost, "/group", "handle="+handle, peer,
		map[string]string{"User-Agent": presenceAgent, "Content-Type": "application/x-www-form-urlencoded"})
	cookies := w.Result().Cookies()
	if w.Code != http.StatusSeeOther || len(cookies) != 1 {
		t.Fatalf("join %q: %d %s", handle, w.Code, w.Body)
	}
	return map[string]string{"User-Agent": presenceAgent, "Cookie": cookies[0].Name + "=" + cookies[0].Value}
}

func TestPresenceDisclosure(t *testing.T) {
	_, a, key := setupPresencePage(t)
	const peer = "192.0.2.71:5000"
	participant := joinPresencePage(t, a, peer, "Ada")
	owner := map[string]string{"X-Facilitator-Key": key}
	a.mu.Lock()
	p := *a.items["group"].Participants["ada@192.0.2.71"]
	a.mu.Unlock()
	if p.Fingerprint == "" || p.LastSeen == "" {
		t.Fatal("fixture must have a fingerprint and last-seen timestamp")
	}

	w := do(a, http.MethodGet, "/group", "", peer, owner)
	for _, want := range []string{"Ada", "anonymous", "Offline", "Last seen", p.Address, p.Fingerprint, `data-hands-action="next"`} {
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), want) {
			t.Errorf("organizer HTML missing %q", want)
		}
	}
	w = do(a, http.MethodGet, "/api/block?id=group&block=attendees", "", peer, owner)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), p.Fingerprint) || !strings.Contains(w.Body.String(), p.Address) {
		t.Fatal("organizer block refresh lost private presence details")
	}

	for _, tc := range []struct {
		name, method, path, action string
		headers                    map[string]string
		status                     int
		want                       []string
	}{
		{"participant HTML", "GET", "/group", "", participant, 200, []string{"Ada", "anonymous", "Offline", "Last seen", `data-hands-action="raise"`}},
		{"organizer preview", "GET", "/group?view=participant", "", owner, 200, []string{"Ada", "anonymous", "Offline", "Last seen"}},
		{"participant block", "GET", "/api/block?id=group&block=attendees", "", participant, 200, []string{"Ada", "anonymous", "Last seen"}},
		{"forged organizer block", "GET", "/api/block?id=group&block=attendees&view=organizer", "", participant, 200, []string{"Ada", "anonymous"}},
		{"preview block", "GET", "/api/block?id=group&block=attendees&view=participant", "", owner, 200, []string{"Ada", "anonymous"}},
		{"display block", "GET", "/api/block?id=group&block=attendees&view=display", "", owner, 200, nil},
		{"display page", "GET", "/group?view=display", "", owner, 200, []string{`id="block-queue"`, "Speaking queue"}},
		{"state", "GET", "/api/state?id=group", "", owner, 200, nil},
		{"show", "POST", "/api/action", `{"action":"show","id":"group"}`, owner, 200, []string{"Ada", "anonymous"}},
		{"preview moderation list", "POST", "/api/action?view=participant", `{"action":"participant","id":"group","kind":"list"}`, owner, 403, nil},
		{"markdown record", "GET", "/api/export?id=group&to=md", "", owner, 200, []string{"Ada", "anonymous", "last seen", "Speaking queue"}},
		{"html record", "GET", "/api/export?id=group&to=html", "", owner, 200, []string{"Ada", "anonymous", "Last seen", "Speaking queue"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := do(a, tc.method, tc.path, tc.action, peer, tc.headers)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.status, response.Body)
			}
			got := response.Body.String()
			for _, private := range []string{p.Address, p.Fingerprint, p.Cookie, key, `data-hands-action="next"`, `data-hands-action="clear"`} {
				if strings.Contains(got, private) {
					t.Fatal("private data or controls in response")
				}
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q", want)
				}
			}
			if tc.name == "display page" && strings.Contains(got, `id="block-backup"`) {
				t.Fatal("an unprojected speaking queue appeared on the display")
			}
			if tc.name == "organizer preview" && strings.Contains(got, `data-hands-action="raise"`) {
				t.Fatal("preview can raise a real hand")
			}
		})
	}
}

func presenceStream(t *testing.T, base, view string, headers map[string]string, status int) (*bufio.Scanner, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/events?id=group&view="+view, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	closeStream := func() {
		cancel()
		resp.Body.Close()
	}
	t.Cleanup(closeStream)
	if resp.StatusCode != status {
		t.Fatalf("stream returned %d, want %d", resp.StatusCode, status)
	}
	return bufio.NewScanner(resp.Body), closeStream
}

func presenceFrame(t *testing.T, stream *bufio.Scanner, block string) {
	t.Helper()
	for stream.Scan() {
		raw, ok := strings.CutPrefix(stream.Text(), "data: ")
		if !ok {
			continue
		}
		var f kit.Frame
		if json.Unmarshal([]byte(raw), &f) != nil || f.Item != "group" || f.Block != block ||
			strings.Contains(raw, "127.0.0.1") || strings.Contains(raw, "River") {
			t.Fatalf("expected a contentless frame for %q, got %s", block, raw)
		}
		return
	}
	t.Fatalf("missing frame for %q: %v", block, stream.Err())
}

func TestPresenceLiveStreams(t *testing.T) {
	base, a, key := setupPresencePage(t)
	a.events.MaxStreams = 4
	owner := map[string]string{"X-Facilitator-Key": key}
	observer, _ := presenceStream(t, base, "participant", nil, 200)
	presenceFrame(t, observer, "")
	checkRosterFrames := func() {
		presenceFrame(t, observer, "attendees")
		presenceFrame(t, observer, "roster")
	}
	checkHandsFrames := func() {
		presenceFrame(t, observer, "queue")
		presenceFrame(t, observer, "backup")
	}
	river := joinPresencePage(t, a, presencePeer, "River")
	checkRosterFrames()
	checkHandsFrames()
	// Use a different page identity on the same address for the rejected stream.
	sky := joinPresencePage(t, a, presencePeer, "Sky")
	checkRosterFrames()
	checkHandsFrames()
	_, closeFirst := presenceStream(t, base, "participant", river, 200)
	checkRosterFrames()
	_, closeSecond := presenceStream(t, base, "participant", river, 200)
	checkRosterFrames()
	_, closePreview := presenceStream(t, base, "participant", owner, 200)
	presenceStream(t, base, "participant", sky, 503)
	closePreview()
	checkPresence := func(handle string, streams int) {
		t.Helper()
		a.mu.Lock()
		defer a.mu.Unlock()
		if len(a.presence) > 1 {
			t.Fatalf("unexpected extra presence groups: %d", len(a.presence))
		}
		group := a.presence[presenceKey{"group", handle + "@127.0.0.1"}]
		if (streams == 0 && group != nil) || (streams > 0 && (group == nil || group.streams != streams)) {
			t.Fatalf("presence for %s does not have %d streams", handle, streams)
		}
	}
	checkPresence("river", 2)
	a.mu.Lock()
	riverBeforeRename := participantRequest{Key: "river@127.0.0.1", Participant: *a.items["group"].Participants["river@127.0.0.1"]}
	a.mu.Unlock()

	r := do(a, "POST", "/api/action?view=participant", `{"action":"hands","id":"group","kind":"raise"}`, presencePeer, river)
	if r.Code != 200 {
		t.Fatalf("raise: %d %s", r.Code, r.Body)
	}
	checkHandsFrames()
	r = ownerAction(t, a, key, `{"action":"participant","id":"group","kind":"rename","source":"River","target":"Reed"}`)
	if r.Code != 200 {
		t.Fatalf("rename: %d %s", r.Code, r.Body)
	}
	checkRosterFrames()
	checkHandsFrames()
	checkPresence("reed", 2)
	lateDisconnect := a.connectPresence("group", riverBeforeRename)
	if lateDisconnect == nil {
		t.Fatal("admission after rename lost the already resolved participant")
	}
	checkRosterFrames()
	checkPresence("reed", 3)
	lateDisconnect()
	checkRosterFrames()
	checkPresence("reed", 2)
	w := do(a, "GET", "/api/block?id=group&block=attendees", "", presencePeer, nil)
	if !strings.Contains(w.Body.String(), "<strong>Reed</strong>") || !strings.Contains(w.Body.String(), " · Online") {
		t.Fatal("renamed participant's open streams did not count")
	}
	a.mu.Lock()
	beforeDisconnect := a.items["group"].Participants["reed@127.0.0.1"].LastSeen
	a.mu.Unlock()

	closeFirst()
	checkRosterFrames()
	checkPresence("reed", 1)
	closeSecond()
	checkRosterFrames()
	checkPresence("reed", 0)
	restart := &app{dir: a.dir, items: map[string]*item{}}
	w = do(restart, "GET", "/api/block?id=group&block=attendees", "", presencePeer, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), " · Online") || !strings.Contains(w.Body.String(), "Last seen") {
		t.Fatal("offline presence or persisted last seen missing after disconnect")
	}
	afterDisconnect := restart.items["group"].Participants["reed@127.0.0.1"].LastSeen
	if afterDisconnect == beforeDisconnect || !strings.Contains(w.Body.String(), afterDisconnect) {
		t.Fatal("recorded last seen did not advance to disconnect")
	}
	w = do(restart, "GET", "/api/block?id=group&block=queue&view=display", "", presencePeer, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<li>Reed</li>") || strings.Contains(w.Body.String(), "data-hands-action") {
		t.Fatal("the persisted renamed hand is missing from the read-only projection")
	}
}

func TestHandsPermissionsAndOrder(t *testing.T) {
	base, a, key := setupPresencePage(t)
	owner := map[string]string{"X-Facilitator-Key": key}
	ada := joinPresencePage(t, a, "192.0.2.31:5000", "Ada")
	bea := joinPresencePage(t, a, "192.0.2.32:5000", "Bea")
	cam := joinPresencePage(t, a, "192.0.2.33:5000", "Cam")
	hands := func(kind, view, peer string, headers map[string]string, status int) {
		t.Helper()
		w := do(a, "POST", "/api/action?view="+view,
			`{"action":"hands","id":"group","kind":"`+kind+`"}`, peer, headers)
		if w.Code != status {
			t.Fatalf("%s as %s: status %d, want %d: %s", kind, view, w.Code, status, w.Body)
		}
		if strings.Contains(w.Body.String(), "192.0.2.") || strings.Contains(w.Body.String(), "participant_key") {
			t.Fatal("private participant data in speaking queue response")
		}
	}
	list := func(want string) {
		t.Helper()
		code, out := cli(base, a)("hands", "list", "group")
		if code != 0 || out != want {
			t.Fatalf("hands list returned %d %q, want %q", code, out, want)
		}
	}

	for _, kind := range []string{"next", "clear", "list"} {
		hands(kind, "organizer", "192.0.2.31:5000", ada, 403)
		hands(kind, "participant", presencePeer, owner, 403)
		hands(kind, "display", presencePeer, owner, 403)
	}
	for _, kind := range []string{"raise", "lower"} {
		hands(kind, "participant", presencePeer, nil, 403)
		hands(kind, "participant", presencePeer, owner, 403)
		hands(kind, "display", "192.0.2.31:5000", ada, 403)
	}
	list("No hands raised.\n")
	hands("raise", "participant", "192.0.2.31:5000", ada, 200)
	hands("raise", "participant", "192.0.2.32:5000", bea, 200)
	hands("raise", "participant", "192.0.2.31:5000", ada, 200)
	hands("raise", "participant", "192.0.2.33:5000", cam, 200)
	list("1. Ada\n2. Bea\n3. Cam\n")
	hands("lower", "participant", "192.0.2.32:5000", bea, 200)
	hands("raise", "participant", "192.0.2.32:5000", bea, 200)
	list("1. Ada\n2. Cam\n3. Bea\n")
	if w := ownerAction(t, a, key, `{"action":"participant","id":"group","kind":"merge","source":"Ada","target":"Bea"}`); w.Code != 200 {
		t.Fatalf("merge: %d %s", w.Code, w.Body)
	}
	list("1. Cam\n2. Bea\n")
	hands("raise", "participant", "192.0.2.31:5000", ada, 403)
	if code, out := cli(base, a)("hands", "next", "group"); code != 0 {
		t.Fatalf("hands next: %d %s", code, out)
	}
	list("1. Bea\n")
	if w := ownerAction(t, a, key, `{"action":"participant","id":"group","kind":"kick","source":"Bea"}`); w.Code != 200 {
		t.Fatalf("kick: %d %s", w.Code, w.Body)
	}
	list("No hands raised.\n")
	hands("raise", "participant", "192.0.2.33:5000", cam, 200)
	for _, format := range []string{"md", "html"} {
		w := do(a, "GET", "/api/export?id=group&to="+format, "", presencePeer, owner)
		entry := "1. Cam"
		if format == "html" {
			entry = "<li>Cam</li>"
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), entry) || strings.Contains(w.Body.String(), "192.0.2.") {
			t.Errorf("export %s lacks the public queue entry or contains private data", format)
		}
	}
	if code, out := cli(base, a)("hands", "clear", "group"); code != 0 {
		t.Fatalf("hands clear: %d %s", code, out)
	}
	list("No hands raised.\n")
}
