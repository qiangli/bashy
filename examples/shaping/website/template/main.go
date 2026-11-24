// site-shaping: the shaping layer for an existing website.
//
// It fronts the public site (served unchanged from its static host), injects
// a small panel into every page, and answers three endpoints under
// _shaping/api/: me (the caller's tier), fields (the private facts their
// audience admits) and view (a generated presentation from only that data).
// Identity comes from outpost's signed relationship stamp; anything unsigned
// is anonymous. A signed-in stranger is a pseudonym, never an address.
//
// It is the server of a shaped app (shaped.json): the owner shapes it with
// `bashy shape`, and `website.bsh serve` runs the approved build for visitors.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"
	"time"
)

//go:embed site.json public fields
var assets embed.FS

// Property declares one visitor-configurable visual or accessibility setting.
// Only values in Values are accepted; entries outside declared Properties are
// refused fail-closed.
type Property struct {
	Values      []string `json:"values,omitempty"`
	CSSVar      string   `json:"css_var,omitempty"`
	ClassPrefix string   `json:"class_prefix,omitempty"`
	Description string   `json:"description,omitempty"`
}

// Wish is one curated, model-free personalization preset offered to all
// visitors (including anonymous) in the panel and applied in localStorage.
type Wish struct {
	ID    string            `json:"id"`
	Label string            `json:"label"`
	Patch map[string]string `json:"patch"`
}

// Config is site.json, shaped source like the rest of this app.
type Config struct {
	SiteURL string `json:"site_url"`
	Title   string `json:"title"`
	Contact string `json:"contact"` // where to ask for more (mailto: or a URL)
	SignIn  string `json:"sign_in"` // sign-in entry on the mount's origin (default /cloud/signin; ?next= is added)
	// StaticOnly serves pages without the site's own scripts that load from
	// it: a client-side router (Next.js, a Vite SPA) sees the engine's mount
	// path, matches no route and renders its 404 over a pre-rendered page.
	// Inline scripts that load nothing (a theme switch) are kept.
	StaticOnly bool `json:"static_only"`
	// ShapeURL is where the owner shapes this layer: the console's Shape page
	// for the registered shaped app, relative to the mount's origin (e.g.
	// /matrix/h/<host>/app/apps/shape/<app>/). Offered to the owner only.
	ShapeURL   string              `json:"shape_url,omitempty"`
	AI         AIRoute             `json:"ai"`
	Views      []View              `json:"views"`
	Properties map[string]Property `json:"properties,omitempty"`
	Wishes     []Wish              `json:"wishes,omitempty"`
}

type server struct {
	cfg     Config
	fields  []Field
	secret  string
	key     string
	version string
	site    *upstream
	public  http.Handler
	allow   allowance
	client  *http.Client
	now     func() time.Time
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "listen address")
	_ = flag.String("data", "", "data directory (unused; views are not stored)")
	siteURL := flag.String("site", "", "override site_url from site.json")
	versionFlag := flag.String("version", "", "override active version id")
	flag.Parse()

	s, err := newServer(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	if *versionFlag != "" {
		s.version = strings.TrimSpace(*versionFlag)
	}
	if *siteURL != "" {
		s.cfg.SiteURL = *siteURL
		s.site = newUpstream(*siteURL)
		s.site.staticOnly = s.cfg.StaticOnly
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	httpSrv := &http.Server{Handler: s}
	log.Printf("site-shaping: fronting %s on %s (version %s, identity verification %v, ai %v)", s.cfg.SiteURL, *addr, shortRev(s.version), s.secret != "", s.key != "" || isLoopbackRoute(s.cfg.AI.BaseURL))

	if f := newShapeFollower(os.Getenv, s.version); f != nil {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		swapped := make(chan struct{})
		var swapErr error
		f.OnSwap = func(bin, rev string, env []string) error {
			log.Printf("site-shaping: switching to approved revision %s", shortRev(rev))
			shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutCancel()
			_ = httpSrv.Shutdown(shutCtx)
			swapErr = replaceProcess(bin, os.Args, env)
			close(swapped)
			return swapErr
		}
		go f.run(ctx)
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
		<-swapped
		if swapErr != nil {
			log.Fatalf("site-shaping: restart failed: %v", swapErr)
		}
		return
	}
	if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func newServer(getenv func(string) string) (*server, error) {
	return newServerFrom(assets, getenv)
}

// newServerFrom builds the server from an asset tree laid out like this app:
// site.json, fields/, public/. Tests pass a fixture instead of the embedded one.
func newServerFrom(fsys fs.FS, getenv func(string) string) (*server, error) {
	raw, err := fs.ReadFile(fsys, "site.json")
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.SignIn == "" {
		// Cloudbox's sign-in entry; the panel adds ?next=<this page> so the
		// visitor lands back here after signing in (also via a satellite).
		cfg.SignIn = "/cloud/signin"
	}
	fields, err := loadFields(fsys, "fields")
	if err != nil {
		return nil, err
	}
	pub, _ := fs.Sub(fsys, "public")
	keyEnv := cfg.AI.KeyEnv
	if keyEnv == "" {
		keyEnv = "SITE_AI_KEY"
	}
	ver := strings.TrimSpace(getenv("SITE_VERSION"))
	if ver == "" {
		ver = computeAssetsVersion(fsys)
	}
	return &server{
		cfg:     cfg,
		fields:  fields,
		secret:  getenv("SITE_SSO_SECRET"),
		key:     getenv(keyEnv),
		version: ver,
		site:    newUpstreamFor(cfg),
		public:  http.StripPrefix("/_shaping/", http.FileServer(http.FS(pub))),
		client:  &http.Client{Timeout: 90 * time.Second},
		now:     time.Now,
	}, nil
}

func isLoopbackRoute(u string) bool {
	return strings.Contains(u, "://127.0.0.1") || strings.Contains(u, "://localhost")
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch p := r.URL.Path; {
	case p == "/_shaping/api/me":
		s.me(w, r)
	case p == "/_shaping/api/version":
		s.versionHandler(w, r)
	case p == "/_shaping/api/fields":
		s.fieldsFor(w, r)
	case p == "/_shaping/api/view":
		s.view(w, r)
	case strings.HasPrefix(p, "/_shaping/"):
		s.public.ServeHTTP(w, r)
	default:
		s.site.ServeHTTP(w, r)
	}
}

func (s *server) versionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "GET version"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"version": s.version})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) allowedFields(c Caller) []Field {
	var out []Field
	for _, f := range s.fields {
		if f.Audience.Admits(c) {
			out = append(out, f)
		}
	}
	return out
}

func (s *server) allowedViews(c Caller) []View {
	if c.Tier == TierAnonymous {
		return nil // no anonymous model calls
	}
	var out []View
	for _, v := range s.views() {
		if v.Audience.Admits(c) {
			out = append(out, v)
		}
	}
	return out
}

func (s *server) views() []View {
	if s.key == "" && !isLoopbackRoute(s.cfg.AI.BaseURL) {
		return nil // no route configured: no views
	}
	return s.cfg.Views
}

// validateWish verifies that every entry in w.Patch targets a declared
// property and one of its declared values, failing closed otherwise.
func validateWish(w Wish, props map[string]Property) error {
	if strings.TrimSpace(w.ID) == "" {
		return fmt.Errorf("wish id is required")
	}
	if strings.TrimSpace(w.Label) == "" {
		return fmt.Errorf("wish %q label is required", w.ID)
	}
	if len(w.Patch) == 0 {
		return fmt.Errorf("wish %q patch must not be empty", w.ID)
	}
	keys := make([]string, 0, len(w.Patch))
	for k := range w.Patch {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, prop := range keys {
		val := w.Patch[prop]
		spec, ok := props[prop]
		if !ok {
			return fmt.Errorf("property %q is not in declared properties", prop)
		}
		if len(spec.Values) == 0 || !slices.Contains(spec.Values, val) {
			return fmt.Errorf("value %q is not allowed for property %q", val, prop)
		}
	}
	return nil
}

func (s *server) allowedWishes() []Wish {
	out := make([]Wish, 0, len(s.cfg.Wishes))
	for _, w := range s.cfg.Wishes {
		if validateWish(w, s.cfg.Properties) == nil {
			out = append(out, w)
		}
	}
	return out
}

type viewRef struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	c := resolveCaller(r, s.secret, s.now())
	var refs []viewRef
	for _, v := range s.allowedViews(c) {
		refs = append(refs, viewRef{v.Name, v.Title})
	}
	props := s.cfg.Properties
	if props == nil {
		props = map[string]Property{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tier":       c.Tier,
		"title":      s.cfg.Title,
		"contact":    s.cfg.Contact,
		"sign_in":    s.cfg.SignIn,
		"fields":     len(s.allowedFields(c)),
		"more":       c.Tier != TierOwner && len(s.allowedFields(c)) < len(s.fields),
		"views":      refs,
		"properties": props,
		"wishes":     s.allowedWishes(),
		"shape":      ownerOnly(c, s.cfg.ShapeURL),
		"version":    s.version,
		// Without the outpost secret nobody can be identified: this is the
		// owner's shaping copy inside the console, not the visitors' engine.
		"public": s.secret != "",
	})
}

func ownerOnly(c Caller, v string) string {
	if c.Tier != TierOwner {
		return ""
	}
	return v
}

func (s *server) fieldsFor(w http.ResponseWriter, r *http.Request) {
	c := resolveCaller(r, s.secret, s.now())
	out := s.allowedFields(c)
	if out == nil {
		out = []Field{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tier": c.Tier, "fields": out})
}

func (s *server) view(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST a view name"})
		return
	}
	c := resolveCaller(r, s.secret, s.now())
	name := r.URL.Query().Get("name")
	var v *View
	for _, cand := range s.allowedViews(c) {
		if cand.Name == name {
			v = &cand
			break
		}
	}
	if v == nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "that view is not available to you"})
		return
	}
	if c.Tier != TierOwner {
		if err := s.allow.take(c.ID(), s.cfg.AI.DailyCap, s.cfg.AI.DailyMax, s.now()); err != nil {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
			return
		}
	}
	page, err := s.site.fetch("/")
	text := ""
	if err == nil {
		text = pageText(page.body, 12000)
	}
	out, err := complete(r.Context(), s.client, s.cfg.AI, s.key, viewPrompt(*v, s.cfg.Title, text, s.allowedFields(c)))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": v.Name, "title": v.Title, "markdown": out})
}
