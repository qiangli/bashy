// Package kit is the shared bashy-desk helper module.
//
// Every desk tool (notepad, form, chart, whiteboard, brainstorm) satisfies
// the same 8-point contract; this package holds the pieces that are
// identical in all of them so tools do not re-implement sessions, ids,
// sharing guards or page furniture. It is stdlib-only, OS-neutral (no
// platform-specific paths or syscalls) and deliberately NOT a yoke package:
// tools are standalone Go modules that pull it in with a relative
// `replace`, which keeps them editable by Software Shaping.
//
// Placement: examples/desk/kit (confirmed in K0). A yoke package would drag
// every tool onto the yoke release train and its dependency closure; a
// shared example module keeps tools standalone, dependency-light and
// forkable per tool or per item template.
package kit

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SchemaVersion is the shared CLI and HTTP output contract.
func SchemaVersion(tool string) string { return "bashy-" + tool + "-v1" }

// Envelope gives every JSON result an object shape and a version. Lists use
// items, while ordinary result fields stay at the top level.
func Envelope(tool string, v any) map[string]any {
	out := map[string]any{"schema_version": SchemaVersion(tool)}
	switch x := v.(type) {
	case nil:
		out["items"] = []any{}
	case map[string]any:
		for k, value := range x {
			out[k] = value
		}
	case []map[string]any:
		out["items"] = x
	default:
		b, _ := json.Marshal(v)
		var object map[string]any
		if json.Unmarshal(b, &object) == nil && object != nil {
			for k, value := range object {
				out[k] = value
			}
		} else {
			out["items"] = v
		}
	}
	return out
}

// Project keeps selected top-level fields and the mandatory version.
func Project(v map[string]any, fields string) (map[string]any, error) {
	if fields == "" {
		return v, nil
	}
	out := map[string]any{"schema_version": v["schema_version"]}
	for _, field := range strings.Split(fields, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			return nil, errors.New("empty field name")
		}
		value, ok := v[field]
		if !ok {
			return nil, errors.New("unknown field " + field)
		}
		out[field] = value
	}
	return out, nil
}

// ErrorMessage reads the common HTTP/CLI error shape.
func ErrorMessage(raw []byte) string {
	var v struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &v) == nil && v.Error.Message != "" {
		return v.Error.Message
	}
	return strings.TrimSpace(string(raw))
}

// ContractHandler applies the same JSON envelope and field projection to
// every API response, including errors. Page and asset bytes pass through.
func ContractHandler(tool string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		next.ServeHTTP(rec, r)
		result := rec.Result()
		defer result.Body.Close()
		body := rec.Body.Bytes()
		if strings.Contains(result.Header.Get("Content-Type"), "application/json") {
			var value any
			if json.Unmarshal(body, &value) == nil {
				v := Envelope(tool, value)
				if message, ok := v["error"].(string); ok {
					code := "request_failed"
					switch result.StatusCode {
					case http.StatusBadRequest:
						code = "invalid_arg"
					case http.StatusNotFound:
						code = "not_found"
					case http.StatusConflict:
						code = "conflict"
					}
					v["error"] = map[string]any{"code": code, "message": message}
				}
				if projected, err := Project(v, r.URL.Query().Get("fields")); err == nil {
					v = projected
				} else {
					result.StatusCode = http.StatusBadRequest
					v = Envelope(tool, map[string]any{"error": map[string]any{"code": "invalid_arg", "message": err.Error()}})
				}
				body, _ = json.Marshal(v)
				body = append(body, '\n')
			}
		}
		for key, values := range result.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.Header().Del("Content-Length")
		w.WriteHeader(result.StatusCode)
		_, _ = w.Write(body)
	})
}

// NameRe is the shared item/session name rule inherited from
// whiteboard/brainstorm: 1-64 chars, letters/digits/-/_ , leading alnum.
var NameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// ValidName reports whether name is an acceptable item id.
func ValidName(name string) bool { return NameRe.MatchString(name) }

// NewToken mints a 48-hex-char capability (participant cookie,
// facilitator/owner key, invite code).
func NewToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// ValidToken reports whether s looks like a minted capability.
func ValidToken(s string) bool {
	if len(s) != 48 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// Grant compares a presented key with the stored one in constant time.
func Grant(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// FacilitatorKey extracts the owner capability from a request.
func FacilitatorKey(r *http.Request) string { return r.Header.Get("X-Facilitator-Key") }

// WriteJSON answers 200 with a JSON body.
func WriteJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// Fail answers with {"error": msg} at the given status.
func Fail(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ApplySecurityHeaders sets the desk-wide response headers every tool must
// send: no caching, no sniffing, no referrer, and a self-only content
// policy that forbids framing (tools are linked/embedded as panels only
// through explicit, tool-owned affordances, never by third-party frames).
func ApplySecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
}

// WithSecurityHeaders wraps h so every response carries ApplySecurityHeaders.
func WithSecurityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ApplySecurityHeaders(w)
		h.ServeHTTP(w, r)
	})
}

// SameOrigin accepts a POST Origin that matches the request host, or the
// public origin forwarded by a loopback-only proxy. Forwarded headers from
// a non-loopback client never weaken the direct check.
func SameOrigin(r *http.Request, u *url.URL) bool {
	if u.Host == r.Host {
		return true
	}
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(remoteHost).IsLoopback() {
		return false
	}
	return u.Host != "" && u.Host == r.Header.Get("X-Forwarded-Host") &&
		u.Scheme == r.Header.Get("X-Forwarded-Proto")
}

// RequestOrigin is the public origin of a request: the Host it arrived on,
// or the origin forwarded by a loopback-only proxy. Forwarded headers from
// a non-loopback client are ignored.
func RequestOrigin(r *http.Request) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && net.ParseIP(remoteHost).IsLoopback() {
		h, p := r.Header.Get("X-Forwarded-Host"), r.Header.Get("X-Forwarded-Proto")
		if h != "" && (p == "http" || p == "https") {
			return p + "://" + h
		}
	}
	return "http://" + r.Host
}

// GuardPOST enforces the desk POST preconditions shared by every tool:
// only GET/POST exist; POST requires a same-origin Origin (when present)
// and refuses cross-site fetch metadata. It writes the refusal and
// returns false when the request must not proceed.
func GuardPOST(w http.ResponseWriter, r *http.Request, pageName string) bool {
	if r.Method != "GET" && r.Method != "POST" {
		Fail(w, http.StatusMethodNotAllowed, "Method not supported")
		return false
	}
	if r.Method == "POST" {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || !SameOrigin(r, u) {
				Fail(w, http.StatusForbidden, "Use the "+pageName+" page")
				return false
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			Fail(w, http.StatusForbidden, "Cross-site request refused")
			return false
		}
	}
	return true
}

// RenderRequest is the POST /api/render body: Markdown in.
type RenderRequest struct {
	Text string `json:"text"`
}

// RenderResponse is the POST /api/render answer: sanitized HTML out.
type RenderResponse struct {
	HTML string `json:"html"`
}

// MaxRenderBytes caps a /api/render body (matches the renderer truncation).
const MaxRenderBytes = 1 << 20

// ServeRender answers the shared Markdown endpoint every tool mounts as
// POST /api/render: Markdown in, sanitized HTML out. render is the tool's
// Markdown func (the shared renderer bound to the item's asset options);
// the kit stays stdlib-only by taking it as a func. Only POST is served
// (anything else is 405); malformed bodies are 400 and renderer failures
// are 500. The endpoint never mutates: it renders text and returns HTML.
func ServeRender(w http.ResponseWriter, r *http.Request, render func(src []byte) ([]byte, error)) {
	if r.Method != "POST" {
		Fail(w, http.StatusMethodNotAllowed, "Method not supported")
		return
	}
	var req RenderRequest
	if !DecodeAction(w, r, MaxRenderBytes, &req) {
		return
	}
	out, err := render([]byte(req.Text))
	if err != nil {
		Fail(w, http.StatusInternalServerError, "Could not render")
		return
	}
	WriteJSON(w, RenderResponse{HTML: string(out)})
}

// DecodeAction decodes exactly one JSON object of at most maxBytes from an
// action POST, rejecting unknown fields and trailing garbage the way both
// reference servers do. It writes the 400 and returns false on any error.
func DecodeAction(w http.ResponseWriter, r *http.Request, maxBytes int64, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		Fail(w, http.StatusBadRequest, "Invalid request")
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		Fail(w, http.StatusBadRequest, "Invalid request")
		return false
	}
	return true
}

// ParticipantCookie returns the caller's stable participant id, minting and
// setting an HttpOnly SameSite=Strict cookie when absent.
func ParticipantCookie(w http.ResponseWriter, r *http.Request, cookieName string) string {
	if c, e := r.Cookie(cookieName); e == nil && ValidToken(c.Value) {
		return c.Value
	}
	id := NewToken()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: id, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 86400 * 30})
	return id
}

// AtomicWrite durably stores data at path: 0700 parent dir, temp file,
// fsync, rename. Readers must copy-modify-save so a failed write is never
// acknowledged or retained.
func AtomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".desk-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// MarshalIndent is json.MarshalIndent with the desk-wide prefix/indent.
func MarshalIndent(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }

// DefaultDataDir resolves $BASHY_HOME/desk/<tool>/, falling back to
// XDG data home and then ~/.local/share, identically on every OS.
func DefaultDataDir(tool string) string {
	base := os.Getenv("BASHY_HOME")
	if base == "" {
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			base = filepath.Join(xdg, "bashy")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				home = "."
			}
			base = filepath.Join(home, ".local", "share", "bashy")
		}
	}
	return filepath.Join(base, "desk", tool)
}

// ItemURL is the stable loopback URL for an item: re-surfacing an item is
// this URL (or `<tool> open <id>`), never a re-emission of its content.
func ItemURL(base, idParam, id string) string {
	return base + "?" + idParam + "=" + url.QueryEscape(id)
}

// OpenView is the contentless re-surface payload every tool returns from
// `open` (and GET /api/state): id, URL and status only — never content.
func OpenView(id, itemURL, status string) map[string]any {
	return map[string]any{"id": id, "url": itemURL, "status": status}
}

// Statuses shared by the UI kit badges and the open/list payloads.
const (
	StatusPending  = "pending"
	StatusNeeds    = "needs-human"
	StatusFilled   = "filled-by-agent"
	StatusAnswered = "answered"
	StatusFailed   = "failed"
	StatusClosed   = "closed"
)

// ValidStatus reports whether s is a known desk status.
func ValidStatus(s string) bool {
	switch s {
	case StatusPending, StatusNeeds, StatusFilled, StatusAnswered, StatusFailed, StatusClosed:
		return true
	}
	return false
}

// secretNameRe matches field names that must never flow through desk:
// credentials stay with `bashy ask`/vault, never in a desk item.
var secretNameRe = regexp.MustCompile(`(?i)password|passwd|secret|credential|api[_-]?key|access[_-]?token|private[_-]?key`)

// Field is one submitted form/questionnaire field.
type Field struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// SecretRefused reports whether any field smuggles a secret: an explicit
// secret type, or a secret-like name, is refused with 400 before storage.
func SecretRefused(fields []Field) bool {
	for _, f := range fields {
		if strings.EqualFold(strings.TrimSpace(f.Type), "secret") {
			return true
		}
		if secretNameRe.MatchString(f.Name) {
			return true
		}
	}
	return false
}

// ErrSecret is the refusal message tools send when SecretRefused fires.
const ErrSecret = "Secret fields are refused here; credentials stay with bashy ask/vault"

// StripPrefix honours a -prefix mount (e.g. /memo) when the tool is served
// behind a sub-path, returning the remainder or false.
func StripPrefix(prefix, path string) (string, bool) {
	if prefix == "" {
		return path, true
	}
	if path == prefix {
		return "/", true
	}
	if strings.HasPrefix(path, prefix+"/") {
		return strings.TrimPrefix(path, prefix), true
	}
	return "", false
}

// CheckLoopback dials nothing: it verifies the configured listen address
// binds loopback, as every new desk tool must (LAN reachability comes only
// through sharing). Whiteboard/brainstorm keep their LAN default as the
// recorded K0 exception; R1 retrofits them.
func CheckLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" {
		return errors.New("desk tools must bind an explicit loopback address, not :port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("desk tools must bind loopback (127.0.0.1 or ::1); got " + host)
	}
	return nil
}

// CanonicalJSON renders v for CLI --json output.
func CanonicalJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSpace(buf.String())
}
