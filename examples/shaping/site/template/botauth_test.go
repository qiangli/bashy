package site

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

// testSigner mirrors bashy's signer (yoke/pkg/webbotauth) with the stdlib
// only, so this module stays standalone.
type testSigner struct {
	key        ed25519.PrivateKey
	kid, tag   string
	agent      string
	created    time.Time
	ttl        time.Duration
	components []string
}

func newTestSigner(t *testing.T) *testSigner {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &testSigner{key: key, kid: JWKThumbprint(key.Public().(ed25519.PublicKey)), tag: "web-bot-auth",
		created: time.Now(), ttl: 5 * time.Minute, components: []string{"@authority", "@method", "@target-uri"}}
}

func (s *testSigner) jwk() TrustedBotKey {
	return TrustedBotKey{Kty: "OKP", Crv: "Ed25519", X: base64.RawURLEncoding.EncodeToString(s.key.Public().(ed25519.PublicKey))}
}

// sign signs r as addressed at target (scheme://authority/path?query).
func (s *testSigner) sign(t *testing.T, r *http.Request, target string) {
	t.Helper()
	components := s.components
	if s.agent != "" {
		r.Header.Set("Signature-Agent", strconv.Quote(s.agent))
		components = append(append([]string{}, components...), "signature-agent")
	}
	params := `("` + strings.Join(components, `" "`) + `");created=` + strconv.FormatInt(s.created.Unix(), 10) +
		";expires=" + strconv.FormatInt(s.created.Add(s.ttl).Unix(), 10) + `;keyid="` + s.kid +
		`";alg="ed25519";nonce="bm9uY2U=";tag="` + s.tag + `"`
	scheme, rest, _ := strings.Cut(target, "://")
	authority, _, _ := strings.Cut(rest, "/")
	var b strings.Builder
	for _, c := range components {
		v := map[string]string{"@authority": authority, "@method": r.Method, "@target-uri": target, "@scheme": scheme}[c]
		if c == "signature-agent" {
			v = r.Header.Get("Signature-Agent")
		}
		b.WriteString(strconv.Quote(c) + ": " + v + "\n")
	}
	b.WriteString(`"@signature-params": ` + params)
	r.Header.Set("Signature-Input", "sig1="+params)
	r.Header.Set("Signature", "sig1=:"+base64.StdEncoding.EncodeToString(ed25519.Sign(s.key, []byte(b.String())))+":")
}

func botSite(t *testing.T, trust string) *Site {
	t.Helper()
	fs := cloneMapFS(fixture)
	cfg := strings.Replace(string(fixture["site.json"].Data), `"owner": "fixture",`, `"owner": "fixture",`+trust, 1)
	fs["site.json"] = &fstest.MapFile{Data: []byte(cfg)}
	s, err := LoadFS(fs)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func trustJSON(t *testing.T, keys []TrustedBotKey, dirs []string, origins ...string) string {
	b, err := json.Marshal(map[string]any{"trusted_bot_keys": keys, "trusted_key_directories": dirs, "trusted_origins": origins})
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(strings.TrimPrefix(string(b), "{"), "}") + ","
}

type served struct {
	status int
	header http.Header
	body   string
}

func serve(h http.Handler, r *http.Request) served {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return served{w.Code, w.Header(), w.Body.String()}
}

// RFC 8037 appendix A.3: the kid bashy signs with is this thumbprint.
func TestJWKThumbprintRFC8037(t *testing.T) {
	x, _ := base64.RawURLEncoding.DecodeString("11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo")
	if got := JWKThumbprint(x); got != "kPrK_qmxVWaYVA9wwBF6Iuo3vVzz7TxHCTwXBygrS4k" {
		t.Fatalf("thumbprint %s", got)
	}
}

// The load-bearing rule: verification adds one header and changes nothing
// else; every failed verification is served exactly like an unsigned one.
func TestVerifiedAgentNoCloaking(t *testing.T) {
	signer := newTestSigner(t)
	other := newTestSigner(t)
	s := botSite(t, trustJSON(t, []TrustedBotKey{signer.jwk()}, nil))
	h := s.Handler()
	const origin = "https://fixture.example"

	for _, tc := range []struct {
		path, accept, method string
	}{
		{"/", "", http.MethodGet},
		{"/about", "text/html", http.MethodGet},
		{"/about", "text/markdown", http.MethodGet},
		{"/about.json", "", http.MethodGet},
		{"/agents.txt", "", http.MethodGet},
		{"/about", "", http.MethodHead},
		{"/missing", "", http.MethodGet},
	} {
		request := func(ua string) *http.Request {
			r := httptest.NewRequest(tc.method, origin+tc.path, nil)
			r.Header.Set("User-Agent", ua)
			if tc.accept != "" {
				r.Header.Set("Accept", tc.accept)
			}
			return r
		}
		base := serve(h, request("bashy-fetch/dev"))
		browser := serve(h, request("Mozilla/5.0"))
		if tc.accept != "" && (browser.body != base.body || browser.header.Get("ETag") != base.header.Get("ETag")) {
			t.Fatalf("%s: browser and agent differ for an explicit Accept", tc.path)
		}
		if base.header.Get(VerifiedAgentHeader) != "" {
			t.Fatalf("%s: unsigned request marked verified", tc.path)
		}

		valid := request("bashy-fetch/dev")
		signer.sign(t, valid, origin+tc.path)
		got := serve(h, valid)
		want := `keyid="` + signer.kid + `"; shape="https://fixture.example/?shape=open"; mcp="https://fixture.example/mcp"`
		if got.header.Get(VerifiedAgentHeader) != want {
			t.Fatalf("%s: verified header %q, want %q", tc.path, got.header.Get(VerifiedAgentHeader), want)
		}
		same := func(name string, g served) {
			t.Helper()
			g.header.Del(VerifiedAgentHeader)
			if g.status != base.status || g.body != base.body || len(g.header) != len(base.header) {
				t.Fatalf("%s %s: response differs from unsigned (status %d/%d)", tc.path, name, g.status, base.status)
			}
			for k := range base.header {
				if g.header.Get(k) != base.header.Get(k) {
					t.Fatalf("%s %s: header %s %q != %q", tc.path, name, k, g.header.Get(k), base.header.Get(k))
				}
			}
		}
		same("valid", got)

		bad := map[string]func(*http.Request){
			"tampered": func(r *http.Request) {
				signer.sign(t, r, origin+tc.path)
				r.Header.Set("Signature", "sig1=:"+base64.StdEncoding.EncodeToString(make([]byte, 64))+":")
			},
			"other-path": func(r *http.Request) { signer.sign(t, r, origin+"/elsewhere") },
			"other-host": func(r *http.Request) { signer.sign(t, r, "http://evil.test"+tc.path) },
			"untrusted":  func(r *http.Request) { other.sign(t, r, origin+tc.path) },
			"expired": func(r *http.Request) {
				old := *signer
				old.created = time.Now().Add(-time.Hour)
				old.sign(t, r, origin+tc.path)
			},
			"future": func(r *http.Request) {
				f := *signer
				f.created = time.Now().Add(10 * time.Minute)
				f.sign(t, r, origin+tc.path)
			},
			"too-long": func(r *http.Request) {
				l := *signer
				l.ttl = 48 * time.Hour
				l.sign(t, r, origin+tc.path)
			},
			"wrong-tag": func(r *http.Request) {
				w := *signer
				w.tag = "other"
				w.sign(t, r, origin+tc.path)
			},
			"no-authority": func(r *http.Request) {
				n := *signer
				n.components = []string{"@method", "@target-uri"}
				n.sign(t, r, origin+tc.path)
			},
			"no-method": func(r *http.Request) {
				n := *signer
				n.components = []string{"@authority", "@target-uri"}
				n.sign(t, r, origin+tc.path)
			},
			"no-target": func(r *http.Request) {
				n := *signer
				n.components = []string{"@authority", "@method"}
				n.sign(t, r, origin+tc.path)
			},
			"oversized": func(r *http.Request) {
				signer.sign(t, r, origin+tc.path)
				r.Header.Add("Signature-Input", "pad=();x=\""+strings.Repeat("x", 4096)+"\"")
			},
			"too-many-members": func(r *http.Request) {
				// The valid sig1 is the fifth member: past the cap.
				signer.sign(t, r, origin+tc.path)
				in := r.Header.Get("Signature-Input")
				for i := 3; i >= 0; i-- {
					label := "pad" + strconv.Itoa(i) + "="
					r.Header.Set("Signature-Input", strings.Replace(in, "sig1=", label, 1)+", "+r.Header.Get("Signature-Input"))
					r.Header.Set("Signature", label+":"+base64.StdEncoding.EncodeToString(make([]byte, 64))+":, "+r.Header.Get("Signature"))
				}
			},
			"agent-uncovered": func(r *http.Request) {
				signer.sign(t, r, origin+tc.path)
				r.Header.Set("Signature-Agent", `"https://agent.example/"`)
			},
		}
		for name, mutate := range bad {
			r := request("bashy-fetch/dev")
			mutate(r)
			g := serve(h, r)
			if g.header.Get(VerifiedAgentHeader) != "" {
				t.Fatalf("%s %s: marked verified", tc.path, name)
			}
			same(name, g)
		}
	}

	// A conditional request revalidates identically: 304 either way.
	etag := serve(h, httptest.NewRequest(http.MethodGet, origin+"/about", nil)).header.Get("ETag")
	r := httptest.NewRequest(http.MethodGet, origin+"/about", nil)
	r.Header.Set("If-None-Match", etag)
	signer.sign(t, r, origin+"/about")
	if g := serve(h, r); g.status != http.StatusNotModified || g.header.Get(VerifiedAgentHeader) == "" {
		t.Fatalf("signed revalidation = %d %q", g.status, g.header.Get(VerifiedAgentHeader))
	}
}

// Behind a front the request is reconstructed under the configured origin
// or a trusted origin (its mount prefix + the request path), never from the
// client-supplied Host or X-Forwarded-* headers.
func TestVerifiedAgentBehindFront(t *testing.T) {
	signer := newTestSigner(t)
	s := botSite(t, trustJSON(t, []TrustedBotKey{signer.jwk()}, nil, "https://dhnt.example/h/host-a/app/site"))
	h := s.Handler()

	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8090/about?x=1", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "dhnt.example")
	r.Header.Set("X-Forwarded-Prefix", "/h/host-a/app/site")
	signer.sign(t, r, "https://dhnt.example/h/host-a/app/site/about?x=1")
	if serve(h, r).header.Get(VerifiedAgentHeader) == "" {
		t.Fatal("trusted-origin request not verified")
	}

	r = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8090/about", nil)
	signer.sign(t, r, "https://fixture.example/about")
	if serve(h, r).header.Get(VerifiedAgentHeader) == "" {
		t.Fatal("origin-addressed request not verified")
	}

	// Forwarded headers alone do not make an origin trusted.
	r = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8090/about", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "other.example")
	r.Header.Set("X-Forwarded-Prefix", "/h/host-a/app/site")
	signer.sign(t, r, "https://other.example/h/host-a/app/site/about")
	if serve(h, r).header.Get(VerifiedAgentHeader) != "" {
		t.Fatal("untrusted forwarded origin verified")
	}
}

// Blocker B1: a valid signature minted for another site, replayed here with
// a forged Host and X-Forwarded-Host naming that site, is not verified and
// is served exactly like an unsigned request.
func TestVerifiedAgentCrossSiteReplay(t *testing.T) {
	signer := newTestSigner(t)
	s := botSite(t, trustJSON(t, []TrustedBotKey{signer.jwk()}, nil))
	h := s.Handler()

	base := serve(h, httptest.NewRequest(http.MethodGet, "https://fixture.example/about", nil))
	for name, forge := range map[string]func(*http.Request){
		"host":      func(r *http.Request) { r.Host = "evil.test" },
		"forwarded": func(r *http.Request) { r.Header.Set("X-Forwarded-Host", "evil.test") },
		"host+forwarded": func(r *http.Request) {
			r.Host = "evil.test"
			r.Header.Set("X-Forwarded-Proto", "https")
			r.Header.Set("X-Forwarded-Host", "evil.test")
		},
	} {
		r := httptest.NewRequest(http.MethodGet, "https://fixture.example/about", nil)
		signer.sign(t, r, "https://evil.test/about")
		forge(r)
		g := serve(h, r)
		if g.header.Get(VerifiedAgentHeader) != "" {
			t.Fatalf("%s: cross-site replay verified", name)
		}
		if g.status != http.StatusOK || g.body != base.body || g.header.Get("ETag") != base.header.Get("ETag") {
			t.Fatalf("%s: replay served differently (status %d)", name, g.status)
		}
	}
}

// Directory keys come only from allowlisted URLs: a Signature-Agent naming
// anything else is never fetched.
func TestVerifiedAgentKeyDirectory(t *testing.T) {
	signer := newTestSigner(t)
	var fetches, strays atomic.Int32
	dir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != SignatureDirectoryPath {
			http.NotFound(w, r)
			return
		}
		fetches.Add(1)
		w.Header().Set("Content-Type", "application/http-message-signatures-directory+json")
		k := signer.jwk()
		k.Kid = signer.kid
		json.NewEncoder(w).Encode(map[string]any{"keys": []TrustedBotKey{k}})
	}))
	defer dir.Close()
	stray := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		strays.Add(1)
		io.WriteString(w, `{"keys":[]}`)
	}))
	defer stray.Close()

	s := botSite(t, trustJSON(t, nil, []string{dir.URL}))
	h := s.Handler()
	const origin = "https://fixture.example"
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest(http.MethodGet, origin+"/about", nil)
		signer.agent = dir.URL + SignatureDirectoryPath
		signer.sign(t, r, origin+"/about")
		if serve(h, r).header.Get(VerifiedAgentHeader) == "" {
			t.Fatalf("request %d: directory key not trusted", i)
		}
	}
	if n := fetches.Load(); n != 1 {
		t.Fatalf("directory fetched %d times, want 1 (cached)", n)
	}

	r := httptest.NewRequest(http.MethodGet, origin+"/about", nil)
	signer.agent = stray.URL + SignatureDirectoryPath
	signer.sign(t, r, origin+"/about")
	if serve(h, r).header.Get(VerifiedAgentHeader) != "" || strays.Load() != 0 {
		t.Fatalf("non-allowlisted Signature-Agent verified or fetched (%d fetches)", strays.Load())
	}

	// Without Signature-Agent the allowlisted directories are consulted.
	r = httptest.NewRequest(http.MethodGet, origin+"/about", nil)
	signer.agent = ""
	signer.sign(t, r, origin+"/about")
	if serve(h, r).header.Get(VerifiedAgentHeader) == "" {
		t.Fatal("allowlisted directory key not found without Signature-Agent")
	}
}

func TestBotTrustValidation(t *testing.T) {
	for name, trust := range map[string]string{
		"bad key":        `"trusted_bot_keys": [{"kty": "OKP", "crv": "Ed25519", "x": "short"}],`,
		"rsa key":        `"trusted_bot_keys": [{"kty": "RSA", "crv": "Ed25519", "x": "11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"}],`,
		"http directory": `"trusted_key_directories": ["http://agent.example/"],`,
		"ftp directory":  `"trusted_key_directories": ["ftp://agent.example/"],`,
		"credentials":    `"trusted_key_directories": ["https://u:p@agent.example/"],`,
		"http origin":    `"trusted_origins": ["http://front.example/site"],`,
		"origin query":   `"trusted_origins": ["https://front.example/site?x=1"],`,
	} {
		fs := cloneMapFS(fixture)
		cfg := strings.Replace(string(fixture["site.json"].Data), `"owner": "fixture",`, `"owner": "fixture",`+trust, 1)
		fs["site.json"] = &fstest.MapFile{Data: []byte(cfg)}
		if _, err := LoadFS(fs); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if u, err := directoryURL("https://Agent.Example"); err != nil || u != "https://agent.example"+SignatureDirectoryPath {
		t.Fatalf("directoryURL = %q %v", u, err)
	}
}

// Directory keys not refreshed for botMaxLifetime are dropped: a revoked key
// fails closed once its directory stops answering.
func TestVerifiedAgentDirectoryStale(t *testing.T) {
	signer := newTestSigner(t)
	var down atomic.Bool
	dir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": []TrustedBotKey{signer.jwk()}})
	}))
	defer dir.Close()

	s := botSite(t, trustJSON(t, nil, []string{dir.URL}))
	h := s.Handler()
	start := time.Now()
	now := start
	s.botVerifier().now = func() time.Time { return now }
	verified := func(at time.Duration) bool {
		now = start.Add(at)
		sg := *signer
		sg.created = now
		r := httptest.NewRequest(http.MethodGet, "https://fixture.example/about", nil)
		sg.sign(t, r, "https://fixture.example/about")
		return serve(h, r).header.Get(VerifiedAgentHeader) != ""
	}
	if !verified(0) {
		t.Fatal("directory key not trusted")
	}
	down.Store(true)
	if !verified(10 * time.Minute) {
		t.Fatal("stale key within the lifetime dropped on a failed refresh")
	}
	if verified(61 * time.Minute) {
		t.Fatal("stale key past the lifetime still trusted")
	}
	down.Store(false)
	if !verified(63 * time.Minute) {
		t.Fatal("key not trusted again after the directory recovered")
	}
}
