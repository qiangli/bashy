package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"desk-kit"
)

func joinRequest(method, path, addr string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = addr
	r.Header.Set("User-Agent", "join-test")
	r.Header.Set("Accept-Language", "en")
	return r
}

func TestJoinCodeAndLinks(t *testing.T) {
	code, err := chooseJoinCode(" Blue Fox ", func(string) bool { return false })
	if err != nil || code != "blue-fox" || pageJoinCode("/"+code) != code {
		t.Fatalf("code = %q, err = %v", code, err)
	}
	if _, err := chooseJoinCode(code, func(string) bool { return true }); err == nil {
		t.Fatal("duplicate code accepted")
	}
	if got, err := chooseJoinCode("", func(string) bool { return false }); err != nil || len(got) != 6 {
		t.Fatalf("generated %q, %v", got, err)
	}

	prefix := "/matrix/h/example/app/page"
	values := url.Values{"code": {"Blue Fox"}}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	joinRoot(response, req, prefix, func(got string) bool { return got == code })
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != prefix+"/"+code {
		t.Fatalf("join redirect: %d %s", response.Code, response.Header().Get("Location"))
	}
	var output strings.Builder
	if err := printJoinLinks(&output, code, "http://example.local:18109", "https://cloud.example.test"+prefix); err != nil {
		t.Fatal(err)
	}
	result := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if result[len(result)-1] != code || !strings.Contains(output.String(), "http://example.local:18109/"+code) ||
		!strings.Contains(output.String(), "https://cloud.example.test"+prefix+"/"+code) ||
		strings.Count(output.String(), "█") < 20 {
		t.Fatalf("missing links, QR codes, or bare code:\n%s", output.String())
	}
}

func TestRefreshAndReclaimKeepOneVote(t *testing.T) {
	now := time.Now()
	page := &PageItem{Votes: map[string]string{}}
	handle := "River"
	first := joinRequest(http.MethodGet, "/blue-fox", "192.0.2.15:9001")
	response := httptest.NewRecorder()
	caller := identifyParticipant(first, "", "", now)
	mintParticipantCookie(response, first, &caller)
	key, err := resolveParticipant(page, caller, &handle, false, now)
	if err != nil || key != "river@192.0.2.15" {
		t.Fatalf("first key %q: %v", key, err)
	}
	page.Votes[key] = "blue"

	refresh := joinRequest(http.MethodGet, "/blue-fox", "192.0.2.15:9002")
	refresh.AddCookie(response.Result().Cookies()[0])
	key, err = resolveParticipant(page, identifyParticipant(refresh, "", "", now), nil, false, now)
	if err != nil {
		t.Fatal(err)
	}
	page.Votes[key] = "green"

	reclaim := joinRequest(http.MethodGet, "/blue-fox", "192.0.2.15:9003")
	caller = identifyParticipant(reclaim, "", "", now)
	mintParticipantCookie(httptest.NewRecorder(), reclaim, &caller)
	key, err = resolveParticipant(page, caller, &handle, false, now)
	if err != nil {
		t.Fatal(err)
	}
	page.Votes[key] = "pink"
	if len(page.Votes) != 1 || len(page.Participants) != 1 || page.Votes[key] != "pink" {
		t.Fatalf("votes %v, participants %d", page.Votes, len(page.Participants))
	}

	other := joinRequest(http.MethodGet, "/blue-fox", "192.0.2.16:9003")
	other.AddCookie(&http.Cookie{Name: pageParticipantCookie, Value: caller.cookie})
	other.Header.Set("X-Forwarded-For", "192.0.2.15")
	if _, err := resolveParticipant(page, identifyParticipant(other, "", "", now), &handle, false, now); !errors.Is(err, errHandleInUse) {
		t.Fatalf("foreign peer error %v", err)
	}
	if _, err := resolveParticipant(page, identifyParticipant(other, "", "", now), nil, false, now); !errors.Is(err, errJoinNeeded) {
		t.Fatalf("foreign cookie error %v", err)
	}
	rows, err := json.Marshal(exportedParticipants(page))
	if err != nil || string(rows) != `[{"handle":"River","auth":"anonymous"}]` {
		t.Fatalf("export %s: %v", rows, err)
	}
}

func signJoinHeaders(r *http.Request, secret string, signedFields []string, signature string) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strings.Join(signedFields, "\n")))
	r.Header.Set(signature, hex.EncodeToString(mac.Sum(nil)))
}

func TestOutpostAddressesAndPrincipal(t *testing.T) {
	now := time.Now()
	const secret = "test-only"
	const prefix = "/matrix/h/example/app/page"
	ts := strconv.FormatInt(now.Unix(), 10)
	r := joinRequest(http.MethodGet, "/blue-fox", "127.0.0.1:9000")
	r.Header.Set("X-Forwarded-Prefix", prefix)
	r.Header.Set("X-Outpost-Identity-Ts", ts)
	r.Header.Set("X-Bashy-Principal", "cloud:forged@example.test")
	signJoinHeaders(r, secret, []string{"", "", prefix, ts}, "X-Outpost-Identity-Sig")
	for _, address := range []string{"192.0.2.34", "192.0.2.35"} {
		r.Header.Set("X-Forwarded-For", "198.51.100.5, "+address+", 127.0.0.1")
		if c := identifyParticipant(r, "", secret, now); c.address != address || c.auth != "anonymous" {
			t.Fatalf("anonymous tunnel caller %+v", c)
		}
	}
	r.RemoteAddr = "192.0.2.36:9000"
	if c := identifyParticipant(r, "", secret, now); c.address != "192.0.2.36" || c.principal != "" {
		t.Fatalf("direct peer %+v", c)
	}
	r.RemoteAddr = "127.0.0.1:9000"
	if c := identifyParticipant(r, "", secret, now.Add(2*time.Minute)); c.address != "127.0.0.1" || c.principal != "" {
		t.Fatalf("stale stamp %+v", c)
	}

	r.Header.Set("X-Outpost-Relationship", "member")
	r.Header.Set("Remote-User", "river@example.test")
	signJoinHeaders(r, secret, []string{"v2", "member", "", "", "", "river@example.test", prefix, ts}, "X-Outpost-Relationship-Sig")
	c := identifyParticipant(r, "", secret, now)
	if c.auth != "member" || c.principal != "cloud:river@example.test" || c.handle != "river@example.test" {
		t.Fatalf("member %+v", c)
	}
	page := &PageItem{}
	if key, err := resolveParticipant(page, c, nil, false, now); err != nil || key != c.principal {
		t.Fatalf("authenticated join %q: %v", key, err)
	}
	r.Header.Set("Remote-User", "forged@example.test")
	if c := identifyParticipant(r, "", secret, now); c.auth != "anonymous" || c.address != "127.0.0.1" {
		t.Fatalf("modified stamp %+v", c)
	}
	if c := identifyParticipant(r, "os:operator", "", now); c.principal != "os:operator" || c.handle != "operator" {
		t.Fatalf("console caller %+v", c)
	}
}

func TestParticipantModeration(t *testing.T) {
	page := &PageItem{Votes: map[string]string{}}
	now := time.Now()
	handle := "River"
	a := participantCaller{auth: "anonymous", address: "192.0.2.1", cookie: kit.NewToken()}
	b := participantCaller{auth: "anonymous", address: "192.0.2.2", cookie: kit.NewToken()}
	first, err := resolveParticipant(page, a, &handle, false, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolveParticipant(page, b, &handle, true, now)
	if err != nil || page.Participants[second].Handle != "River-2" {
		t.Fatalf("suffix %q: %v", second, err)
	}
	page.Votes[first], page.Votes[second] = "A", "B"
	if err := moderateParticipant(page, "rename", first, "Lake"); err != nil {
		t.Fatal(err)
	}
	first = anonymousKey("Lake", a.address)
	if err := moderateParticipant(page, "merge", second, first); err != nil || len(page.Votes) != 1 || page.Votes[first] != "A" {
		t.Fatalf("merge votes %v: %v", page.Votes, err)
	}
	if _, err := resolveParticipant(page, b, nil, false, now); !errors.Is(err, errKicked) {
		t.Fatalf("merged participant got %v", err)
	}
	if err := moderateParticipant(page, "kick", first, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveParticipant(page, a, nil, false, now); !errors.Is(err, errKicked) {
		t.Fatalf("kicked participant got %v", err)
	}
	if err := moderateParticipant(page, "open", "", ""); err != nil || !page.OpenHandles {
		t.Fatalf("open handles: %v", err)
	}
}

func TestJoinHTTPRoutesAndExport(t *testing.T) {
	_, a := startServer(t, "")
	response := do(a, http.MethodPost, "/api/action",
		`{"action":"add","id":"long-page-id","title":"Workshop","body":"Welcome to the workshop.","audience":"lan","join_code":"Blue Fox"}`,
		"127.0.0.1:5000", nil)
	var created struct{ OwnerKey string }
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &created) != nil || created.OwnerKey == "" {
		t.Fatalf("create: %d %s", response.Code, response.Body)
	}
	response = do(a, http.MethodPost, "/api/action", `{"action":"add","id":"other","join_code":"blue-fox"}`, "127.0.0.1:5000", nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate code returned %d", response.Code)
	}
	form := func(path, addr string, values url.Values, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
		r.RemoteAddr = addr
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	const client = "192.0.2.19:5001"
	response = form("/", client, url.Values{"code": {"blue-fox"}}, nil)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/blue-fox" {
		t.Fatalf("code form: %d %s", response.Code, response.Header().Get("Location"))
	}
	response = do(a, http.MethodGet, "/blue-fox", "", client, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Name or email") ||
		strings.Contains(response.Body.String(), "Welcome to the workshop.") {
		t.Fatalf("unauthenticated visit: %d", response.Code)
	}
	// The join form posts natively: its screen must let the browser send a
	// real same-origin Origin, and a null Origin stays refused.
	if got := response.Header().Get("Referrer-Policy"); got != "same-origin" {
		t.Fatalf("join screen Referrer-Policy %q, want same-origin", got)
	}
	if response = form("/blue-fox", client, url.Values{"handle": {"River"}}, map[string]string{"Origin": "null"}); response.Code != http.StatusForbidden {
		t.Fatalf("null-origin join: %d", response.Code)
	}
	name := url.Values{"handle": {"River"}}
	response = form("/blue-fox", client, name, map[string]string{"Origin": "http://example.com"})
	cookies := response.Result().Cookies()
	if response.Code != http.StatusSeeOther || len(cookies) != 1 {
		t.Fatalf("first join: %d, cookies %d", response.Code, len(cookies))
	}
	response = form("/blue-fox", "192.0.2.20:5002", name, map[string]string{"X-Forwarded-For": "192.0.2.19"})
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate handle from a different address: %d", response.Code)
	}
	response = do(a, http.MethodGet, "/?id=long-page-id", "", client, map[string]string{"Cookie": cookies[0].Name + "=" + cookies[0].Value})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Welcome to the workshop.") {
		t.Fatalf("legacy visit did not render the page: %d", response.Code)
	}
	response = do(a, http.MethodGet, "/api/export?id=long-page-id&to=html", "", "127.0.0.1:5000",
		map[string]string{"X-Facilitator-Key": created.OwnerKey})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "River") ||
		strings.Contains(response.Body.String(), "192.0.2.19") || strings.Contains(response.Body.String(), cookies[0].Value) {
		t.Fatalf("export status or participant record: %d", response.Code)
	}
}

func TestMissingTunnelAddressRefusesAnonymousJoin(t *testing.T) {
	now := time.Now()
	const secret = "test-only"
	const prefix = "/matrix/h/example/app/page"
	ts := strconv.FormatInt(now.Unix(), 10)
	r := joinRequest(http.MethodGet, "/blue-fox", "127.0.0.1:9000")
	r.Header.Set("X-Forwarded-Prefix", prefix)
	r.Header.Set("X-Outpost-Identity-Ts", ts)
	signJoinHeaders(r, secret, []string{"", "", prefix, ts}, "X-Outpost-Identity-Sig")
	c := identifyParticipant(r, "", secret, now)
	handle := "River"
	if _, err := resolveParticipant(&PageItem{}, c, &handle, false, now); !errors.Is(err, errTunnelAddress) {
		t.Fatalf("missing tunnel address returned %v", err)
	}
}

func TestParticipantTracingUsesOwnerEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	p := Participant{Handle: "River", Auth: "anonymous", Address: "192.0.2.19"}
	r := httptest.NewRequest(http.MethodGet, "/blue-fox", nil)
	traceParticipant(r, p, time.Now())()
	if r.Header.Get("traceparent") != "" {
		t.Fatal("tracing active without owner configuration")
	}
	received := make(chan string, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body strings.Builder
		_, _ = io.Copy(&body, r.Body)
		received <- r.URL.Path + " " + body.String()
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	traceParticipant(r, p, time.Now())()
	select {
	case got := <-received:
		for _, want := range []string{"/v1/traces", "participant.handle", "participant.auth", "participant.address", p.Handle, p.Address} {
			if !strings.Contains(got, want) {
				t.Fatalf("trace payload lacks %q", want)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no trace reached the configured endpoint")
	}
}

func TestPublishUsesFixedPageListener(t *testing.T) {
	var out strings.Builder
	code := runCLI([]string{"publish-outpost", "--port", "18111", "--dry-run", "--json"}, strings.NewReader(""), &out)
	if code != 0 || !strings.Contains(out.String(), "127.0.0.1:18111/") || !strings.Contains(out.String(), "--trust-cloud-identity") {
		t.Fatalf("publish: exit %d, %s", code, out.String())
	}
	if err := bindGuard("0.0.0.0:0", true, io.Discard); err == nil {
		t.Fatal("accepted an ephemeral LAN port")
	}
	if err := bindGuard("0.0.0.0:18111", true, io.Discard); err != nil {
		t.Fatalf("fixed LAN port: %v", err)
	}
	if joinPolicy(pageManifest) || !joinPolicy([]byte(`{"server":{"args":["serve","--handle-conflicts=suffix"]}}`)) {
		t.Fatal("handle policy was not read from the shaped server args")
	}
}

func TestJoinedRequestUsesSameRespondentAfterRefresh(t *testing.T) {
	p := &PageItem{Votes: map[string]string{}}
	opts := pageJoinOptions{}
	mutate := func(fn func(*PageItem) error) error { return fn(p) }
	vote := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		participant, ok := requestParticipant(r)
		if !ok {
			t.Fatal("missing participant")
		}
		p.Votes[participant.Key] = "yes"
		w.WriteHeader(http.StatusNoContent)
	})
	const path = "/blue-fox"
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("handle=River"))
	req.RemoteAddr = "192.0.2.19:5000"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	joined := httptest.NewRecorder()
	opts.pageJoin(joined, req, mutate, "Workshop", path, "/", true, vote)
	cookies := joined.Result().Cookies()
	if joined.Code != http.StatusSeeOther || len(cookies) != 1 {
		t.Fatalf("join returned %d, %d cookies", joined.Code, len(cookies))
	}
	for _, addr := range []string{"192.0.2.19:5000", "192.0.2.19:5001"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = addr
		req.AddCookie(cookies[0])
		w := httptest.NewRecorder()
		opts.pageJoin(w, req, mutate, "Workshop", path, "/", true, vote)
		if w.Code != http.StatusNoContent {
			t.Fatalf("vote returned %d", w.Code)
		}
	}
	if len(p.Votes) != 1 || p.Votes["river@192.0.2.19"] != "yes" {
		t.Fatalf("votes = %v", p.Votes)
	}
}
