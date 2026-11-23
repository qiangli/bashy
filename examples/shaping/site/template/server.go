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

func (s *Site) BashyShapedValue(p *Page) string {
	rev := ""
	if p != nil {
		rev = p.Revision
	} else {
		rev = s.SiteRevision()
	}
	rel := func(v string) string {
		if s.Config.Origin != "" && strings.HasPrefix(v, s.Config.Origin) {
			if r := strings.TrimPrefix(v, s.Config.Origin); r != "" {
				return r
			}
			return "/"
		}
		return v
	}
	return `module="` + s.Config.Module + `", revision="` + rev + `", agents="/agents.txt", skill="` + rel(s.Config.Skill) + `", mcp="` + rel(s.Config.MCP) + `", source="` + s.Config.Source + `", shape="` + rel(s.Config.Shape) + `"`
}
func (s *Site) LinkHeaderValue(p *Page) string {
	if p == nil {
		p = s.homePage()
	}
	if p == nil {
		return ""
	}
	mdTwin, jsonTwin := TwinPaths(p.Path)
	canonicalURL := s.Config.Origin + p.Path
	parts := []string{
		`<` + mdTwin + `>; rel="alternate"; type="text/markdown"`,
		`<` + jsonTwin + `>; rel="alternate"; type="application/json"`,
		`</llms.txt>; rel="describedby"`,
		`<` + s.EditURL(p) + `>; rel="edit"`,
	}
	if s.Config.Source != "" {
		parts = append(parts, `<`+s.Config.Source+`>; rel="via"`)
	}
	parts = append(parts,
		`</AGENTS.md>; rel="service-doc"`,
		`</.well-known/mcp/server-card.json>; rel="service-desc"`,
		`<`+canonicalURL+`>; rel="canonical"`,
		`</humans.txt>; rel="author"`,
		`</llms.txt>; rel="help"`,
	)
	return strings.Join(parts, ", ")
}
func (h *routeHandler) setAgentHeaders(w http.ResponseWriter, p *Page) {
	w.Header().Set("Bashy-Shaped", h.site.BashyShapedValue(p))
	if v := h.site.LinkHeaderValue(p); v != "" {
		w.Header().Set("Link", v)
	}
	w.Header().Set("Vary", "Accept")
	if p != nil {
		w.Header().Set("ETag", `"`+p.Revision+`"`)
	} else if rev := h.site.SiteRevision(); rev != "" {
		w.Header().Set("ETag", `"`+rev+`"`)
	}
}
func (h *routeHandler) serveText(w http.ResponseWriter, r *http.Request, ctype, body string) {
	h.setAgentHeaders(w, nil)
	w.Header().Set("Content-Type", ctype)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte(body))
	}
}
func (h *routeHandler) serveError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	h.setAgentHeaders(w, nil)
	if status == http.StatusMethodNotAllowed {
		w.Header().Set("Allow", "GET, HEAD")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		home := h.site.homePage()
		note := `<p id="agent-note" role="note">This site is agent-ready: instructions at #agents - see /agents.txt and /llms.txt</p>`
		if home != nil {
			if info := h.site.AgentInfoFor(home); info != nil {
				note = `<p id="agent-note" role="note">This site is agent-ready: instructions at #agents - MCP ` + info.MCP + ` - skill ` + info.Skill + `</p>`
			}
		}
		_, _ = w.Write([]byte("<!DOCTYPE html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>" + msg + "</title></head><body>\n" + note + "\n<h1>" + msg + "</h1><p><a href=\"/llms.txt\">Site index</a></p></body></html>\n"))
	}
}
func IsAgentUserAgent(ua string, list []string) bool {
	lower := strings.ToLower(ua)
	for _, a := range list {
		if a != "" && strings.Contains(lower, strings.ToLower(a)) {
			return true
		}
	}
	return false
}

func (h *routeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.serveError(w, r, http.StatusMethodNotAllowed, "read-only")
		return
	}

	// Verified bashy agents get a header naming the agent-only entry points;
	// the response itself is identical either way (no cloaking).
	if kid, ok := h.site.VerifyAgent(r); ok {
		w.Header().Set(VerifiedAgentHeader, h.site.verifiedAgentValue(kid))
	}

	reqPath := r.URL.Path
	if reqPath == "" {
		reqPath = "/"
	}

	switch reqPath {
	case "/agents.txt":
		h.serveText(w, r, "text/plain; charset=utf-8", h.site.RenderAgentsTxt())
		return
	case "/AGENTS.md":
		h.serveText(w, r, "text/markdown; charset=utf-8", h.site.RenderAGENTSmd())
		return
	case "/README.md":
		h.serveText(w, r, "text/markdown; charset=utf-8", h.site.RenderReadmeMD())
		return
	case "/skill.md":
		h.serveText(w, r, "text/markdown; charset=utf-8", h.site.RenderSkillMD())
		return
	case "/humans.txt":
		h.serveText(w, r, "text/plain; charset=utf-8", h.site.RenderHumansTxt())
		return
	case "/robots.txt":
		h.serveText(w, r, "text/plain; charset=utf-8", h.site.RenderRobotsTxt())
		return
	case "/llms.txt":
		h.serveText(w, r, "text/plain; charset=utf-8", h.site.RenderLLMsTxt())
		return
	case "/sitemap.xml":
		h.serveText(w, r, "application/xml; charset=utf-8", h.site.RenderSitemapXML())
		return
	case "/feed.xml":
		h.serveText(w, r, "application/atom+xml; charset=utf-8", h.site.RenderAtomFeed())
		return
	case "/.well-known/agent-skills/index.json":
		h.serveText(w, r, "application/json; charset=utf-8", h.site.RenderAgentSkillsIndex())
		return
	case "/.well-known/mcp/server-card.json":
		h.serveText(w, r, "application/json; charset=utf-8", h.site.RenderMCPServerCard())
		return
	case "/.well-known/oauth-protected-resource":
		h.serveText(w, r, "application/json; charset=utf-8", h.site.RenderOAuthProtectedResource())
		return
	case "/.well-known/agent-card.json":
		h.serveText(w, r, "application/json; charset=utf-8", h.site.RenderAgentCard())
		return
	case "/.well-known/api-catalog":
		h.serveText(w, r, "application/linkset+json; charset=utf-8", h.site.RenderAPICatalog())
		return
	case "/.well-known/security.txt":
		h.serveText(w, r, "text/plain; charset=utf-8", h.site.RenderSecurityTxt())
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
		h.serveError(w, r, http.StatusNotFound, "404 page not found")
		return
	}
	format := NegotiateRequest(r)
	if IsAgentUserAgent(r.UserAgent(), h.site.Config.AgentUserAgents) {
		accept := strings.TrimSpace(strings.Join(r.Header.Values("Accept"), ", "))
		if accept == "" || accept == "*/*" {
			format = "md"
		}
	}
	h.serveRepresentation(w, r, p, format)
}

func (h *routeHandler) serveRepresentation(w http.ResponseWriter, r *http.Request, p *Page, format string) {
	etag := `"` + p.Revision + `"`
	h.setAgentHeaders(w, p)

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
	order       int
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
	if a.specificity != b.specificity {
		return a.specificity > b.specificity
	}
	return a.order < b.order
}
func (a acceptSpec) equalRank(b acceptSpec) bool {
	return a.matched && b.matched && a.q == b.q && a.specificity == b.specificity && a.order == b.order
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

	for idx, part := range strings.Split(accept, ",") {
		order := idx
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
			cand := acceptSpec{q: q, specificity: spec, order: order, matched: true}
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

	if mdSpec.equalRank(htmlSpec) || jsonSpec.equalRank(htmlSpec) {
		// Complete tie (e.g. */*): HTML default.
		if mdSpec.equalRank(jsonSpec) {
			return "html"
		}
	}
	if mdSpec.betterThan(htmlSpec) && !jsonSpec.betterThan(mdSpec) {
		return "md"
	}
	if jsonSpec.betterThan(htmlSpec) && jsonSpec.betterThan(mdSpec) {
		return "json"
	}
	return "html"
}
