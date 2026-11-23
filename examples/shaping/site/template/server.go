package site

import (
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

type routeHandler struct {
	site *Site
}

// NewHandler returns an http.Handler serving the public routes for an
// already-loaded Site: canonical HTML pages, Accept content negotiation,
// .md and .json twins, /robots.txt, /llms.txt, Vary: Accept, strong ETag,
// Link alternates, and strong If-None-Match 304 responses.
func NewHandler(s *Site) http.Handler {
	return &routeHandler{site: s}
}

// Handler returns an http.Handler serving the public routes for s.
func (s *Site) Handler() http.Handler {
	return NewHandler(s)
}

// NewHandlerFS loads and validates a Site from fsys and returns an
// http.Handler serving its public routes.
func NewHandlerFS(fsys fs.FS) (http.Handler, error) {
	_, h, err := NewServerHandler(fsys, nil)
	return h, err
}

// NewServerHandler loads a Site from fsys, applies SITE_ORIGIN when getenv is
// non-nil and returns a non-empty value, validates structural invariants, and
// returns both the loaded Site and its public routes http.Handler.
func NewServerHandler(fsys fs.FS, getenv func(string) string) (*Site, http.Handler, error) {
	s, err := LoadFS(fsys)
	if err != nil {
		return nil, nil, err
	}
	if getenv != nil {
		if o := strings.TrimSpace(getenv("SITE_ORIGIN")); o != "" {
			s.SetOrigin(o)
		}
	}
	if err := Validate(s); err != nil {
		return nil, nil, err
	}
	return s, NewHandler(s), nil
}

func (h *routeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "read-only", http.StatusMethodNotAllowed)
		return
	}

	reqPath := r.URL.Path
	if reqPath == "" {
		reqPath = "/"
	}

	switch reqPath {
	case "/robots.txt":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(h.site.RenderRobotsTxt()))
		}
		return
	case "/llms.txt":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(h.site.RenderLLMsTxt()))
		}
		return
	case "/index.md":
		if p, ok := h.site.ByPath["/"]; ok {
			h.serveRepresentation(w, r, p, "md")
			return
		}
	case "/index.json":
		if p, ok := h.site.ByPath["/"]; ok {
			h.serveRepresentation(w, r, p, "json")
			return
		}
	}

	if strings.HasSuffix(reqPath, ".md") {
		base := strings.TrimSuffix(reqPath, ".md")
		if base != "" && base != "/index" {
			if p, ok := h.site.ByPath[base]; ok {
				h.serveRepresentation(w, r, p, "md")
				return
			}
		}
	}
	if strings.HasSuffix(reqPath, ".json") {
		base := strings.TrimSuffix(reqPath, ".json")
		if base != "" && base != "/index" {
			if p, ok := h.site.ByPath[base]; ok {
				h.serveRepresentation(w, r, p, "json")
				return
			}
		}
	}

	p, ok := h.site.ByPath[reqPath]
	if !ok {
		http.NotFound(w, r)
		return
	}
	h.serveRepresentation(w, r, p, NegotiateRequest(r))
}

func (h *routeHandler) serveRepresentation(w http.ResponseWriter, r *http.Request, p *Page, format string) {
	mdTwin, jsonTwin := TwinPaths(p.Path)
	canonicalURL := h.site.Config.Origin + p.Path
	etag := `"` + p.Revision + `"`

	w.Header().Set("Vary", "Accept")
	w.Header().Set("ETag", etag)
	w.Header().Set("Link",
		`<`+mdTwin+`>; rel="alternate"; type="text/markdown", `+
			`<`+jsonTwin+`>; rel="alternate"; type="application/json", `+
			`<`+canonicalURL+`>; rel="canonical"`)

	var (
		ctype string
		body  []byte
	)
	switch format {
	case "md":
		ctype = "text/markdown; charset=utf-8"
		body = []byte(p.RenderMarkdown())
	case "json":
		ctype = "application/json; charset=utf-8"
		var err error
		body, err = p.RenderJSON()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		ctype = "text/html; charset=utf-8"
		body = []byte(h.site.RenderHTMLDocument(p))
	}
	w.Header().Set("Content-Type", ctype)

	if ifNoneMatchMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}

func ifNoneMatchMatches(header, strongETag string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	for _, raw := range strings.Split(header, ",") {
		tok := strings.TrimSpace(raw)
		if strings.HasPrefix(tok, "W/") {
			continue // strong comparison only
		}
		if tok == strongETag {
			return true
		}
	}
	return false
}

type acceptSpec struct {
	q           float64
	specificity int // 3 = exact type/subtype, 2 = type/*, 1 = */*
	matched     bool
}

func (a acceptSpec) betterThan(b acceptSpec) bool {
	if !a.matched {
		return false
	}
	if !b.matched {
		return true
	}
	if a.q != b.q {
		return a.q > b.q
	}
	return a.specificity > b.specificity
}

// NegotiateRequest chooses "html" (default), "md", or "json" from r's Accept
// header values.
func NegotiateRequest(r *http.Request) string {
	if r == nil {
		return "html"
	}
	return NegotiateAccept(strings.Join(r.Header.Values("Accept"), ", "))
}

// NegotiateAccept chooses "html" (default), "md" (text/markdown), or "json"
// (application/json) per RFC 9110 quality and specificity rules.
func NegotiateAccept(accept string) string {
	accept = strings.TrimSpace(accept)
	if accept == "" {
		return "html"
	}
	var htmlSpec, mdSpec, jsonSpec acceptSpec
	var htmlDisallowed, mdDisallowed, jsonDisallowed bool

	for _, part := range strings.Split(accept, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments := strings.Split(part, ";")
		mediaType := strings.ToLower(strings.TrimSpace(segments[0]))
		q := 1.0
		for _, param := range segments[1:] {
			k, v, ok := strings.Cut(strings.TrimSpace(param), "=")
			if ok && strings.EqualFold(strings.TrimSpace(k), "q") {
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					if parsed < 0 {
						parsed = 0
					}
					if parsed > 1 {
						parsed = 1
					}
					q = parsed
				}
			}
		}
		apply := func(cur *acceptSpec, disallowed *bool, spec int) {
			if q == 0 {
				if spec == 3 {
					*disallowed = true
				}
				return
			}
			cand := acceptSpec{q: q, specificity: spec, matched: true}
			if cand.betterThan(*cur) {
				*cur = cand
			}
		}
		switch mediaType {
		case "text/html", "application/xhtml+xml":
			apply(&htmlSpec, &htmlDisallowed, 3)
		case "text/markdown", "text/x-markdown":
			apply(&mdSpec, &mdDisallowed, 3)
		case "application/json":
			apply(&jsonSpec, &jsonDisallowed, 3)
		case "text/*":
			apply(&htmlSpec, &htmlDisallowed, 2)
			apply(&mdSpec, &mdDisallowed, 2)
		case "application/*":
			apply(&jsonSpec, &jsonDisallowed, 2)
		case "*/*":
			apply(&htmlSpec, &htmlDisallowed, 1)
			apply(&mdSpec, &mdDisallowed, 1)
			apply(&jsonSpec, &jsonDisallowed, 1)
		}
	}

	if htmlDisallowed {
		htmlSpec = acceptSpec{}
	}
	if mdDisallowed {
		mdSpec = acceptSpec{}
	}
	if jsonDisallowed {
		jsonSpec = acceptSpec{}
	}

	// HTML is the default unless Markdown or JSON is strictly preferred.
	if mdSpec.betterThan(htmlSpec) && !jsonSpec.betterThan(mdSpec) {
		return "md"
	}
	if jsonSpec.betterThan(htmlSpec) && jsonSpec.betterThan(mdSpec) {
		return "json"
	}
	return "html"
}
