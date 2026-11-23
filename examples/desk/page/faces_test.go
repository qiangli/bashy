package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPageFaceDisclosure(t *testing.T) {
	base, a := startServer(t, "")
	const (
		brainstormKey = "test-brainstorm-facilitator-key"
		queueKey      = "test-queue-organizer-key"
		projectorKey  = "test-queue-projector-key"
		boardURL      = "/brainstorm/?session=ideation"
		organizerURL  = boardURL + "#facilitator=" + brainstormKey
		queueURL      = "http://demo.example/control#" + queueKey
		projectorURL  = "http://demo.example/projector#" + projectorKey
	)
	tmp := t.TempDir()
	bodyPath := filepath.Join(tmp, "agenda.md")
	body := "# Workshop\n\nWelcome to the workshop.\n\n" +
		"```block embed:" + boardURL + " id=brainstorm\n```\n\n" +
		"```block embed id=queue\n```\n"
	if err := os.WriteFile(bodyPath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(tmp, "faces.json")
	config, err := json.Marshal(map[string]faceSettings{
		"brainstorm": {OrganizerURL: organizerURL},
		"queue":      {OrganizerURL: queueURL, DisplayURL: projectorURL, LinkOnly: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatal(err)
	}
	code, out := cli(base, a)("add", "workshop", "-f", bodyPath, "--faces", configPath, "--code", "roles",
		"--audience", "lan", "--json")
	var created struct{ OrganizerURL, OwnerKey string }
	if code != 0 || json.Unmarshal([]byte(out), &created) != nil || created.OwnerKey == "" {
		t.Fatal("could not create workshop through the CLI")
	}
	entryURL, err := url.Parse(created.OrganizerURL)
	if err != nil || entryURL.Path != "/roles" || entryURL.Query().Get("view") != "organizer" ||
		entryURL.Fragment != "owner="+created.OwnerKey {
		t.Fatal("add did not return a separate organizer entry link")
	}

	// Start with an empty cache to verify that the faces come from the
	// persisted item and still obey the same projection after restart.
	a = &app{dir: a.dir, baseURL: base, items: map[string]*item{}}
	const peer = "192.0.2.50:5000"
	owner := map[string]string{"X-Facilitator-Key": created.OwnerKey}
	signIn := `{"action":"organizer-session","id":"workshop"}`
	browser := do(a, "POST", "/api/action", signIn, peer, owner)
	cookies := browser.Result().Cookies()
	if browser.Code != 200 || len(cookies) != 1 {
		t.Fatalf("organizer session returned %d with %d cookies", browser.Code, len(cookies))
	}
	if do(a, "POST", "/api/action", signIn, peer, nil).Code != http.StatusForbidden {
		t.Fatal("organizer session accepted without owner key")
	}
	var organizerHeaders map[string]string
	for _, cookie := range cookies {
		if strings.HasPrefix(cookie.Name, "page-organizer-") {
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Value == created.OwnerKey {
				t.Fatal("organizer cookie did not isolate the raw page key")
			}
			organizerHeaders = map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}
		}
	}
	if organizerHeaders == nil {
		t.Fatal("no organizer cookie")
	}

	organizer := do(a, "GET", "/roles?view=organizer", "", peer, organizerHeaders)
	view := organizer.Body.String()
	for _, part := range []string{organizerURL, boardURL, queueURL, projectorURL,
		`role="tab"`, `>Organizer</button>`, `>Participant</button>`, `>Display</button>`, "View as participant"} {
		if organizer.Code != 200 || !strings.Contains(view, part) {
			t.Errorf("organizer face missing %q", part)
		}
	}
	if strings.Contains(view, created.OwnerKey) {
		t.Fatal("raw page key in organizer HTML")
	}

	req := httptest.NewRequest("POST", "/roles", strings.NewReader("handle=River"))
	req.RemoteAddr = peer
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	join := httptest.NewRecorder()
	a.ServeHTTP(join, req)
	participantCookies := join.Result().Cookies()
	if join.Code != http.StatusSeeOther || len(participantCookies) != 1 {
		t.Fatalf("participant join: status %d, cookies %d", join.Code, len(participantCookies))
	}
	participant := map[string]string{"Cookie": participantCookies[0].Name + "=" + participantCookies[0].Value}
	for _, tt := range []struct {
		name, method, path, body string
		headers                  map[string]string
		status                   int
	}{
		{"participant HTML", "GET", "/roles", "", participant, 200},
		{"participant state", "GET", "/api/state?id=workshop&view=organizer", "", participant, 200},
		{"forged organizer view", "GET", "/roles?view=organizer", "", participant, 200},
		{"participant show", "POST", "/api/action", `{"action":"show","id":"workshop"}`, participant, 200},
		{"participant HTML export", "GET", "/api/export?id=workshop&to=html", "", participant, 403},
		{"participant Markdown export", "GET", "/api/export?id=workshop&to=md", "", participant, 403},
		{"organizer show", "POST", "/api/action", `{"action":"show","id":"workshop"}`, owner, 200},
		{"organizer HTML export", "GET", "/api/export?id=workshop&to=html", "", owner, 200},
		{"organizer Markdown export", "GET", "/api/export?id=workshop&to=md", "", owner, 200},
		{"organizer preview", "GET", "/roles?view=participant", "", organizerHeaders, 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := do(a, tt.method, tt.path, tt.body, peer, tt.headers)
			if response.Code != tt.status {
				t.Fatalf("status %d, want %d", response.Code, tt.status)
			}
			got := response.Body.String()
			for _, forbidden := range []string{brainstormKey, queueKey, projectorKey, created.OwnerKey, `>Organizer</button>`, "Open organizer view", "/control#"} {
				if strings.Contains(got, forbidden) {
					t.Fatal("organizer data leaked")
				}
			}
			if tt.name == "participant HTML" || tt.name == "organizer preview" {
				if !strings.Contains(got, boardURL) || !strings.Contains(got, "Welcome to the workshop.") {
					t.Fatal("participant content missing")
				}
			}
			if tt.name == "organizer preview" {
				preview := response.Result().Cookies()
				if len(preview) != 1 || preview[0].Name != "page-preview-workshop" ||
					!strings.Contains(got, "Return to organizer view") {
					t.Fatal("preview did not use its own browser identity")
				}
			}
		})
	}
	projector := do(a, "GET", "/roles?view=display", "", peer, nil)
	got := projector.Body.String()
	if projector.Code != 200 || !strings.Contains(got, `href="`+projectorURL+`"`) || !strings.Contains(got, `class="desk-main page-display"`) {
		t.Fatal("projector view missing")
	}
	for _, omitted := range []string{brainstormKey, queueKey, boardURL, "<button", "Welcome to the workshop.", "allow-forms"} {
		if strings.Contains(got, omitted) {
			t.Errorf("projector HTML includes %q", omitted)
		}
	}
	if len(a.items["workshop"].Participants) != 1 {
		t.Fatal("preview or display registered a real participant")
	}
}

func TestFaceSettingsFollowBlockReference(t *testing.T) {
	_, a := startServer(t, "")
	ownerKey := addPage(t, a, "p", "host")
	const peer = "127.0.0.1:5000"
	owner := map[string]string{"X-Facilitator-Key": ownerKey}
	set := func(body string, private map[string]any, headers map[string]string) int {
		raw, _ := json.Marshal(map[string]any{"action": "set", "id": "p", "body": body, "faces": private})
		return do(a, "POST", "/api/action", string(raw), peer, headers).Code
	}
	const block = "```block embed:/board id=room\n```\n"
	faces := map[string]any{"room": faceSettings{OrganizerURL: "/control#private-demo-key", DisplayURL: "/projector#display-demo-key"}}
	if set(block, faces, nil) != 403 {
		t.Fatal("participant changed face settings")
	}
	if set(strings.ReplaceAll(block, "/board", "/board#private-demo-key"), faces, owner) != 400 ||
		set(block+"private-demo-key", faces, owner) != 400 ||
		set(block, map[string]any{"room": faceSettings{OrganizerURL: "javascript:alert(1)"}}, owner) != 400 {
		t.Fatal("accepted invalid or leaked face configuration")
	}
	if set(block, faces, owner) != 200 || set("# Revised\n"+block, nil, owner) != 200 {
		t.Fatal("could not store and retain face settings")
	}
	if !strings.Contains(do(a, "GET", "/?id=p", "", peer, owner).Body.String(), "/control#private-demo-key") {
		t.Fatal("unrelated Markdown edit discarded settings")
	}
	if set(strings.ReplaceAll(block, "/board", "/another"), nil, owner) != 200 ||
		strings.Contains(do(a, "GET", "/?id=p", "", peer, owner).Body.String(), "private-demo-key") {
		t.Fatal("replacing a public reference did not discard its prior private settings")
	}
}
