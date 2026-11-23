package site

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Verified bashy agents (Web Bot Auth). A request signed per RFC 9421 with
// tag="web-bot-auth", alg="ed25519" by a key the owner trusts in site.json
// (trusted_bot_keys, or a key listed by an allowlisted
// trusted_key_directories URL) is answered with a Bashy-Verified-Agent header
// naming the agent-only entry points. Verification never changes content:
// body, ETag and status are identical for signed, unsigned and failed
// requests; an invalid signature is simply not marked verified.
//
// The signed @authority is bound to the site's own audience: the configured
// origin and the owner's trusted_origins, never the client-supplied Host or
// X-Forwarded-* headers, so a signature minted for another site does not
// verify here.

// VerifiedAgentHeader is the response header a verified request receives.
const VerifiedAgentHeader = "Bashy-Verified-Agent"

// SignatureDirectoryPath is the well-known key directory path.
const SignatureDirectoryPath = "/.well-known/http-message-signatures-directory"

const (
	botClockSkew     = 60 * time.Second
	botMaxLifetime   = time.Hour
	botDirTTL        = 5 * time.Minute
	botDirRetry      = time.Minute
	botDirTimeout    = 5 * time.Second
	botDirBodyLimit  = 64 << 10
	botDirMaxEntries = 16
	botSigHeaderMax  = 4096
	botSigMaxMembers = 4
)

// TrustedBotKey is an Ed25519 public key as an OKP JWK; an empty kid is the
// RFC 7638 thumbprint, which is the keyid bashy signs with.
type TrustedBotKey struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Kid string `json:"kid,omitempty"`
}

func (k TrustedBotKey) public() (string, ed25519.PublicKey, error) {
	if k.Kty != "OKP" || k.Crv != "Ed25519" {
		return "", nil, fmt.Errorf("want kty OKP, crv Ed25519")
	}
	x, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(k.X, "="))
	if err != nil || len(x) != ed25519.PublicKeySize {
		return "", nil, fmt.Errorf("x is not a base64url Ed25519 public key")
	}
	kid := k.Kid
	if kid == "" {
		kid = JWKThumbprint(x)
	}
	return kid, ed25519.PublicKey(x), nil
}

// JWKThumbprint is the RFC 7638 SHA-256 thumbprint of an Ed25519 key.
func JWKThumbprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256([]byte(`{"crv":"Ed25519","kty":"OKP","x":"` + base64.RawURLEncoding.EncodeToString(pub) + `"}`))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// validateBotTrust checks the site.json trust configuration.
func validateBotTrust(c *Config) error {
	for i, k := range c.TrustedBotKeys {
		if _, _, err := k.public(); err != nil {
			return fmt.Errorf("site.json: trusted_bot_keys[%d]: %v", i, err)
		}
	}
	if len(c.TrustedOrigins) > botDirMaxEntries {
		return fmt.Errorf("site.json: trusted_origins: at most %d", botDirMaxEntries)
	}
	for i, o := range c.TrustedOrigins {
		if _, err := originView(o, true); err != nil {
			return fmt.Errorf("site.json: trusted_origins[%d]: %v", i, err)
		}
	}
	if len(c.TrustedKeyDirectories) > botDirMaxEntries {
		return fmt.Errorf("site.json: trusted_key_directories: at most %d", botDirMaxEntries)
	}
	for i, d := range c.TrustedKeyDirectories {
		if _, err := directoryURL(d); err != nil {
			return fmt.Errorf("site.json: trusted_key_directories[%d]: %v", i, err)
		}
	}
	return nil
}

// directoryURL normalizes a key directory URL: https (http only on
// loopback), no credentials or fragment; a bare origin means the
// well-known path.
func directoryURL(raw string) (string, error) {
	u, err := secureURL(raw)
	if err != nil {
		return "", err
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = SignatureDirectoryPath
	}
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

// secureURL parses an absolute https URL (http only on loopback) without
// credentials or fragment.
func secureURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("want an absolute https URL without credentials")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if ip := net.ParseIP(u.Hostname()); u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, errors.New("http is allowed for loopback only")
		}
	default:
		return nil, errors.New("want an https URL")
	}
	return u, nil
}

type botDirectory struct {
	mu      sync.Mutex
	keys    map[string]ed25519.PublicKey
	fetched time.Time
	tried   time.Time
}

// botVerifier holds the trusted keys and the directory cache.
type botVerifier struct {
	static map[string]ed25519.PublicKey
	dirs   map[string]*botDirectory
	order  []string
	client *http.Client
	now    func() time.Time
}

func newBotVerifier(c Config) *botVerifier {
	v := &botVerifier{
		static: map[string]ed25519.PublicKey{},
		dirs:   map[string]*botDirectory{},
		now:    time.Now,
		client: &http.Client{
			Timeout: botDirTimeout,
			// Only the allowlisted URL is ever fetched: no redirects.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	for _, k := range c.TrustedBotKeys {
		if kid, pub, err := k.public(); err == nil {
			v.static[kid] = pub
		}
	}
	for _, d := range c.TrustedKeyDirectories {
		if u, err := directoryURL(d); err == nil && v.dirs[u] == nil {
			v.dirs[u] = &botDirectory{}
			v.order = append(v.order, u)
		}
	}
	return v
}

func (v *botVerifier) enabled() bool { return len(v.static) > 0 || len(v.dirs) > 0 }

// lookup returns the trusted key for kid: static keys first, then the
// directories, restricted to the one Signature-Agent names when it names an
// allowlisted directory, and none at all when it names another.
func (v *botVerifier) lookup(kid, agent string) ed25519.PublicKey {
	if pub := v.static[kid]; pub != nil {
		return pub
	}
	candidates := v.order
	if agent != "" {
		u, err := directoryURL(agent)
		if err != nil || v.dirs[u] == nil {
			return nil
		}
		candidates = []string{u}
	}
	for _, u := range candidates {
		if pub := v.directoryKey(u, kid); pub != nil {
			return pub
		}
	}
	return nil
}

// directoryKey returns kid from the directory at u, refreshing a stale or
// missing entry at most once per botDirRetry without holding the lock over
// the fetch. Keys not refreshed for botMaxLifetime are dropped, so a revoked
// key fails closed once its directory stops serving it or goes away.
func (v *botVerifier) directoryKey(u, kid string) ed25519.PublicKey {
	d := v.dirs[u]
	now := v.now()
	d.mu.Lock()
	fresh := !d.fetched.IsZero() && now.Sub(d.fetched) < botDirTTL
	if pub := d.keys[kid]; pub != nil && fresh {
		d.mu.Unlock()
		return pub
	}
	refresh := now.Sub(d.tried) >= botDirRetry
	if refresh {
		d.tried = now
	}
	d.mu.Unlock()
	if refresh {
		keys, err := v.fetchDirectory(u)
		d.mu.Lock()
		if err == nil && !d.fetched.After(now) {
			d.keys, d.fetched = keys, now
		}
		d.mu.Unlock()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fetched.IsZero() || v.now().Sub(d.fetched) >= botMaxLifetime {
		d.keys = nil
		return nil
	}
	return d.keys[kid]
}

func (v *botVerifier) fetchDirectory(u string) (map[string]ed25519.PublicKey, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/http-message-signatures-directory+json, application/json;q=0.9")
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, botDirBodyLimit+1))
	if err != nil || len(body) > botDirBodyLimit {
		return nil, fmt.Errorf("%s: unreadable or oversized directory", u)
	}
	var set struct {
		Keys []TrustedBotKey `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, err
	}
	keys := map[string]ed25519.PublicKey{}
	for _, k := range set.Keys {
		if kid, pub, err := k.public(); err == nil && kid == JWKThumbprint(pub) {
			keys[kid] = pub
		}
	}
	return keys, nil
}

// botSignature is one parsed Signature-Input member.
type botSignature struct {
	label      string
	components []string
	params     map[string]string // string values unquoted, integers as text
	raw        string            // the serialized member value, for @signature-params
}

// verify reports the trusted keyid of a valid web-bot-auth signature on r,
// addressed at one of the site's origins.
func (v *botVerifier) verify(r *http.Request, origins []string) (string, bool) {
	input := strings.Join(r.Header.Values("Signature-Input"), ", ")
	signature := strings.Join(r.Header.Values("Signature"), ", ")
	if len(input) > botSigHeaderMax || len(signature) > botSigHeaderMax {
		return "", false
	}
	inputs := parseSignatureInput(input)
	if len(inputs) == 0 {
		return "", false
	}
	if len(inputs) > botSigMaxMembers {
		inputs = inputs[:botSigMaxMembers]
	}
	sigs := parseSignatureValues(signature)
	views := requestViews(r, origins)
	agent := signatureAgentURL(r.Header.Values("Signature-Agent"))
	now := v.now().Unix()
	for _, in := range inputs {
		p := in.params
		if p["tag"] != "web-bot-auth" || p["alg"] != "ed25519" || p["keyid"] == "" {
			continue
		}
		created, err1 := strconv.ParseInt(p["created"], 10, 64)
		expires, err2 := strconv.ParseInt(p["expires"], 10, 64)
		if err1 != nil || err2 != nil || created > now+int64(botClockSkew/time.Second) ||
			expires <= now || expires <= created || expires-created > int64(botMaxLifetime/time.Second) {
			continue
		}
		covered := map[string]bool{}
		for _, c := range in.components {
			covered[c] = true
		}
		// Bound to the site, method and target: a signature cannot be
		// replayed on another path, method or Signature-Agent.
		if !covered["@authority"] || !covered["@method"] || (!covered["@target-uri"] && !covered["@path"]) ||
			(r.Header.Get("Signature-Agent") != "" && !covered["signature-agent"]) {
			continue
		}
		sig := sigs[in.label]
		if len(sig) != ed25519.SignatureSize {
			continue
		}
		pub := v.lookup(p["keyid"], agent)
		if pub == nil {
			continue
		}
		for _, view := range views {
			base, ok := signatureBase(r, view, in)
			if ok && ed25519.Verify(pub, []byte(base), sig) {
				return p["keyid"], true
			}
		}
	}
	return "", false
}

// requestView is the request as the client addressed it.
type requestView struct{ scheme, authority, path, query string }

// originView parses a site origin (scheme://authority[/mount prefix]); a
// trusted origin must also be https, or http on loopback.
func originView(origin string, secure bool) (requestView, error) {
	var u *url.URL
	var err error
	if secure {
		u, err = secureURL(origin)
	} else {
		u, err = url.Parse(strings.TrimSpace(origin))
	}
	if err != nil {
		return requestView{}, err
	}
	if u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.RawQuery != "" || u.Fragment != "" {
		return requestView{}, errors.New("want scheme://host[/prefix] without query or fragment")
	}
	return requestView{scheme: u.Scheme, authority: normalizeAuthority(u.Scheme, u.Host), path: strings.TrimRight(u.EscapedPath(), "/")}, nil
}

// requestViews reconstructs the public request under each configured
// origin: its scheme and authority, its mount prefix + the request path.
// Host and X-Forwarded-* are client-supplied and never consulted, so the
// signed @authority must name this site.
func requestViews(r *http.Request, origins []string) []requestView {
	path := r.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	var views []requestView
	for _, o := range origins {
		view, err := originView(o, false)
		if err != nil {
			continue
		}
		view.path += path
		view.query = r.URL.RawQuery
		if !slices.Contains(views, view) {
			views = append(views, view)
		}
	}
	return views
}

func normalizeAuthority(scheme, host string) string {
	host = strings.ToLower(host)
	if h, port, err := net.SplitHostPort(host); err == nil &&
		((port == "443" && scheme == "https") || (port == "80" && scheme == "http")) {
		if strings.Contains(h, ":") {
			return "[" + h + "]"
		}
		return h
	}
	return host
}

func signatureBase(r *http.Request, view requestView, in botSignature) (string, bool) {
	var b strings.Builder
	for _, c := range in.components {
		var value string
		switch c {
		case "@authority":
			value = view.authority
		case "@method":
			value = r.Method
		case "@scheme":
			value = view.scheme
		case "@path":
			value = view.path
		case "@query":
			value = "?" + view.query
		case "@target-uri":
			value = view.scheme + "://" + view.authority + view.path
			if view.query != "" {
				value += "?" + view.query
			}
		default:
			if strings.HasPrefix(c, "@") || c != strings.ToLower(c) {
				return "", false
			}
			values := r.Header.Values(c)
			if len(values) == 0 {
				return "", false
			}
			for i := range values {
				values[i] = strings.TrimSpace(values[i])
			}
			value = strings.Join(values, ", ")
		}
		b.WriteString(strconv.Quote(c) + ": " + value + "\n")
	}
	b.WriteString(`"@signature-params": ` + in.raw)
	return b.String(), true
}

// splitMembers splits a structured-field dictionary on top-level commas.
func splitMembers(s string) []string {
	var out []string
	depth, quoted, start := 0, false, 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case quoted && c == '\\':
			i++
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			out = append(out, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// parseSignatureInput parses `label=("c1" "c2");k=v;...` members.
func parseSignatureInput(header string) []botSignature {
	var out []botSignature
	for _, member := range splitMembers(header) {
		label, value, ok := strings.Cut(member, "=")
		if !ok || !strings.HasPrefix(value, "(") {
			continue
		}
		end := strings.IndexByte(value, ')')
		if end < 0 {
			continue
		}
		sig := botSignature{label: strings.TrimSpace(label), params: map[string]string{}, raw: value}
		for _, item := range strings.Fields(value[1:end]) {
			c, err := strconv.Unquote(item)
			if err != nil {
				sig.components = nil
				break
			}
			sig.components = append(sig.components, c)
		}
		for _, param := range strings.Split(value[end+1:], ";")[1:] {
			k, v, _ := strings.Cut(param, "=")
			if unq, err := strconv.Unquote(v); err == nil {
				v = unq
			}
			sig.params[strings.TrimSpace(k)] = v
		}
		if len(sig.components) > 0 {
			out = append(out, sig)
		}
	}
	return out
}

// parseSignatureValues parses `label=:base64:` members.
func parseSignatureValues(header string) map[string][]byte {
	out := map[string][]byte{}
	for _, member := range splitMembers(header) {
		label, value, ok := strings.Cut(member, "=")
		if !ok || len(value) < 2 || value[0] != ':' || value[len(value)-1] != ':' {
			continue
		}
		if raw, err := base64.StdEncoding.DecodeString(value[1 : len(value)-1]); err == nil {
			out[strings.TrimSpace(label)] = raw
		}
	}
	return out
}

// signatureAgentURL reads Signature-Agent as an sf-string ("https://...")
// or a one-member dictionary (label="https://...").
func signatureAgentURL(values []string) string {
	v := strings.TrimSpace(strings.Join(values, ", "))
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, `"`) {
		if _, rest, ok := strings.Cut(v, "="); ok {
			v = strings.TrimSpace(rest)
		}
	}
	if unq, err := strconv.Unquote(v); err == nil {
		return unq
	}
	return v
}

// verifiedAgentValue is the Bashy-Verified-Agent header value.
func (s *Site) verifiedAgentValue(kid string) string {
	return `keyid="` + kid + `"; shape="` + s.Config.Shape + `"; mcp="` + s.Config.MCP + `"`
}

func (s *Site) botVerifier() *botVerifier {
	s.botOnce.Do(func() { s.bot = newBotVerifier(s.Config) })
	return s.bot
}

// VerifyAgent reports the trusted keyid when r carries a valid Web Bot Auth
// signature from a key this site trusts.
func (s *Site) VerifyAgent(r *http.Request) (string, bool) {
	v := s.botVerifier()
	if !v.enabled() || r.Header.Get("Signature-Input") == "" {
		return "", false
	}
	return v.verify(r, append([]string{s.Config.Origin}, s.Config.TrustedOrigins...))
}
