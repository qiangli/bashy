// Package site loads Markdown pages with declared {#id} section identifiers
// and renders deterministic HTML fragments/documents, Markdown twins, and JSON
// under one canonical content SHA-256 revision, with an http.Handler for the
// public routes and structural/compare-and-set checks.
package site

import (
	"bufio"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Assets holds the module's embedded site.json and content/*.md files so an
// external module importing this repository at a pinned version can render and
// serve its published content without a separate filesystem checkout.
//
//go:embed site.json content
var Assets embed.FS

// Config is site.json: site-wide metadata and shared navigation.
type Config struct {
	Title  string    `json:"title"`
	Origin string    `json:"origin"`
	Owner  string    `json:"owner,omitempty"`
	Nav    []NavItem `json:"nav"`
}

// NavItem is one link in the shared navigation bar.
type NavItem struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

// Section is one rendered content section with a declared identifier.
type Section struct {
	ID       string `json:"id"`
	Heading  string `json:"heading"`
	HTML     string `json:"html"`
	Markdown string `json:"markdown"`
	SHA256   string `json:"sha256"`
}

// Page is one rendered content document across HTML, Markdown, and JSON.
type Page struct {
	Path         string    `json:"path"`
	Title        string    `json:"title"`
	Summary      string    `json:"summary,omitempty"`
	Revision     string    `json:"revision"`
	Nav          []NavItem `json:"nav"`
	Sections     []Section `json:"sections"`
	Source       string    `json:"-"`
	DeclaredList []string  `json:"-"`
}

// Site holds the loaded configuration and pages in deterministic order.
type Site struct {
	Config Config
	Pages  []*Page
	ByPath map[string]*Page
}

var sectionIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Load loads site.json and content/*.md from a filesystem directory.
func Load(dir string) (*Site, error) {
	return LoadFS(os.DirFS(dir))
}

// LoadSiteDir is an alias for Load.
func LoadSiteDir(dir string) (*Site, error) {
	return Load(dir)
}

// LoadEmbedded loads the site from the package's embedded Assets.
func LoadEmbedded() (*Site, error) {
	return LoadFS(Assets)
}

// LoadSite is an alias for LoadFS.
func LoadSite(fsys fs.FS) (*Site, error) {
	return LoadFS(fsys)
}

// LoadFS loads site.json and content/*.md from fsys without failing on
// check-level invariants (duplicate ids or broken links), so individual check
// commands can report their specific diagnostics. Call Validate to enforce all
// structural checks before serving.
func LoadFS(fsys fs.FS) (*Site, error) {
	rawCfg, err := fs.ReadFile(fsys, "site.json")
	if err != nil {
		return nil, fmt.Errorf("read site.json: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(rawCfg, &cfg); err != nil {
		return nil, fmt.Errorf("parse site.json: %w", err)
	}
	cfg.Title = strings.TrimSpace(cfg.Title)
	cfg.Origin = strings.TrimRight(strings.TrimSpace(cfg.Origin), "/")
	if cfg.Title == "" {
		return nil, errors.New("site.json: title is required")
	}
	if cfg.Origin == "" {
		return nil, errors.New("site.json: origin is required")
	}
	if cfg.Nav == nil {
		cfg.Nav = []NavItem{}
	}

	entries, err := fs.ReadDir(fsys, "content")
	if err != nil {
		return nil, fmt.Errorf("read content/: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, errors.New("content/: at least one .md page is required")
	}

	site := &Site{
		Config: cfg,
		ByPath: map[string]*Page{},
	}
	for _, name := range names {
		rel := path.Join("content", name)
		raw, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		p, err := parsePage(rel, string(raw), cfg.Nav)
		if err != nil {
			return nil, err
		}
		site.Pages = append(site.Pages, p)
		if _, exists := site.ByPath[p.Path]; !exists {
			site.ByPath[p.Path] = p
		}
	}

	sort.SliceStable(site.Pages, func(i, j int) bool {
		if site.Pages[i].Path == site.Pages[j].Path {
			return site.Pages[i].Source < site.Pages[j].Source
		}
		if site.Pages[i].Path == "/" {
			return true
		}
		if site.Pages[j].Path == "/" {
			return false
		}
		return site.Pages[i].Path < site.Pages[j].Path
	})
	return site, nil
}

// SetOrigin updates the canonical origin used in HTML links and llms.txt.
// Revisions depend only on canonical content, never on deployment origin.
func (s *Site) SetOrigin(origin string) {
	s.Config.Origin = strings.TrimRight(strings.TrimSpace(origin), "/")
}

// TwinPaths returns the .md and .json twin paths for a canonical page path.
func TwinPaths(pagePath string) (mdPath, jsonPath string) {
	if pagePath == "/" {
		return "/index.md", "/index.json"
	}
	clean := strings.TrimRight(pagePath, "/")
	return clean + ".md", clean + ".json"
}

// parsePage parses front matter (required title and path; optional summary and
// sections; unknown key: value lines are ignored, malformed lines missing ':'
// are rejected) and body sections with declared {#id} identifiers.
func parsePage(source, raw string, nav []NavItem) (*Page, error) {
	norm := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(norm, "---\n") {
		return nil, fmt.Errorf("%s: missing front matter (expected --- block with title and path)", source)
	}
	end := strings.Index(norm[4:], "\n---")
	if end < 0 {
		return nil, fmt.Errorf("%s: unterminated front matter", source)
	}
	front := norm[4 : 4+end]
	rest := norm[4+end+4:]
	if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}

	p := &Page{
		Source: source,
		Nav:    append([]NavItem{}, nav...),
	}
	sc := bufio.NewScanner(strings.NewReader(front))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("%s: invalid front matter line %q", source, line)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(unquoteYAML(strings.TrimSpace(v)))
		switch k {
		case "title":
			p.Title = v
		case "path":
			p.Path = v
		case "summary":
			p.Summary = v
		case "sections":
			for _, item := range strings.Split(v, ",") {
				if id := strings.TrimSpace(item); id != "" {
					p.DeclaredList = append(p.DeclaredList, id)
				}
			}
		}
	}
	if p.Title == "" {
		return nil, fmt.Errorf("%s: front matter missing title", source)
	}
	if p.Path == "" || !strings.HasPrefix(p.Path, "/") {
		return nil, fmt.Errorf("%s: front matter path %q must start with /", source, p.Path)
	}
	if p.Path != "/" {
		p.Path = path.Clean(p.Path)
	}

	sections, err := parseSections(source, rest)
	if err != nil {
		return nil, err
	}
	if len(sections) == 0 {
		return nil, fmt.Errorf("%s: page must declare at least one ## Heading {#id} section", source)
	}
	if len(p.DeclaredList) > 0 {
		var got []string
		for _, sec := range sections {
			got = append(got, sec.ID)
		}
		if !slices.Equal(got, p.DeclaredList) {
			return nil, fmt.Errorf("%s: front matter sections %v do not match body sections %v", source, p.DeclaredList, got)
		}
	}
	p.Sections = sections
	p.Revision = ComputeRevision(p)
	return p, nil
}

func unquoteYAML(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		inner := s[1 : len(s)-1]
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

func parseSections(source, body string) ([]Section, error) {
	lines := strings.Split(body, "\n")
	type rawSection struct {
		id      string
		heading string
		lines   []string
	}
	var raw []rawSection
	var fence string

	for lineNo, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, string(fence[0])) == "" {
				fence = ""
			}
			if len(raw) == 0 {
				return nil, fmt.Errorf("%s:%d: content appears before first ## Heading {#id} section", source, lineNo+1)
			}
			raw[len(raw)-1].lines = append(raw[len(raw)-1].lines, line)
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			ch := trimmed[0]
			n := 0
			for n < len(trimmed) && trimmed[n] == ch {
				n++
			}
			fence = trimmed[:n]
			if len(raw) == 0 {
				return nil, fmt.Errorf("%s:%d: code block appears before first ## Heading {#id} section", source, lineNo+1)
			}
			raw[len(raw)-1].lines = append(raw[len(raw)-1].lines, line)
			continue
		}
		if strings.HasPrefix(line, "## ") || line == "##" {
			heading, id, err := parseSectionHeading(source, lineNo+1, strings.TrimSpace(strings.TrimPrefix(line, "##")))
			if err != nil {
				return nil, err
			}
			raw = append(raw, rawSection{id: id, heading: heading})
			continue
		}
		if strings.HasPrefix(line, "# ") {
			return nil, fmt.Errorf("%s:%d: unexpected # heading in body (page title comes from front matter; sections use ## Heading {#id})", source, lineNo+1)
		}
		if len(raw) == 0 {
			if trimmed != "" {
				return nil, fmt.Errorf("%s:%d: content appears before first ## Heading {#id} section", source, lineNo+1)
			}
			continue
		}
		raw[len(raw)-1].lines = append(raw[len(raw)-1].lines, line)
	}
	if fence != "" {
		return nil, fmt.Errorf("%s: unclosed code fence %s", source, fence)
	}

	out := make([]Section, 0, len(raw))
	for _, r := range raw {
		md := canonicalMarkdownBlock(r.lines)
		out = append(out, Section{
			ID:       r.id,
			Heading:  r.heading,
			HTML:     renderMarkdownBlocks(md),
			Markdown: md,
			SHA256:   ComputeSectionHash(r.id, r.heading, md),
		})
	}
	return out, nil
}

func parseSectionHeading(source string, lineNo int, text string) (heading, id string, err error) {
	open := strings.LastIndex(text, "{#")
	if open < 0 || !strings.HasSuffix(text, "}") {
		return "", "", fmt.Errorf("%s:%d: section heading %q lacks a declared {#id}; section ids are never derived from headings", source, lineNo, text)
	}
	heading = strings.TrimSpace(text[:open])
	id = strings.TrimSpace(text[open+2 : len(text)-1])
	if heading == "" {
		return "", "", fmt.Errorf("%s:%d: section heading with {#%s} has empty title", source, lineNo, id)
	}
	if !sectionIDPattern.MatchString(id) {
		return "", "", fmt.Errorf("%s:%d: invalid section id %q (must match %s)", source, lineNo, id, sectionIDPattern.String())
	}
	return heading, id, nil
}

func canonicalMarkdownBlock(lines []string) string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if start >= end {
		return ""
	}
	cleaned := make([]string, 0, end-start)
	for _, l := range lines[start:end] {
		cleaned = append(cleaned, strings.TrimRight(l, " \t"))
	}
	return strings.Join(cleaned, "\n")
}

// ComputeSectionHash returns the 64-hex-character SHA-256 digest of a section's
// canonical id, heading, and Markdown body.
func ComputeSectionHash(id, heading, markdown string) string {
	sum := sha256.Sum256([]byte("site-section-v1\nid:" + id + "\nheading:" + heading + "\n" + markdown))
	return hex.EncodeToString(sum[:])
}

// CanonicalDocument builds the canonical byte representation of a page's
// content. Copy changes (title, summary, path, nav, section ids/headings/markdown)
// change the revision; HTML/CSS/header/origin rendering changes do not.
func CanonicalDocument(p *Page) []byte {
	var b strings.Builder
	b.WriteString("site-content-v1\n")
	b.WriteString("path:" + p.Path + "\n")
	b.WriteString("title:" + p.Title + "\n")
	if p.Summary != "" {
		b.WriteString("summary:" + p.Summary + "\n")
	}
	for _, n := range p.Nav {
		b.WriteString("nav:" + n.Label + "\t" + n.Href + "\n")
	}
	for _, s := range p.Sections {
		b.WriteString("section:" + s.ID + "\n")
		b.WriteString("heading:" + s.Heading + "\n")
		b.WriteString("markdown:\n" + s.Markdown + "\n---\n")
	}
	return []byte(b.String())
}

// ComputeRevision returns the 12-hex-character short SHA-256 content revision
// of p's CanonicalDocument.
func ComputeRevision(p *Page) string {
	sum := sha256.Sum256(CanonicalDocument(p))
	return hex.EncodeToString(sum[:6])
}

// RenderHTMLFragment renders the semantic HTML fragment for p.
func RenderHTMLFragment(p *Page) string {
	return p.RenderHTMLFragment()
}

// RenderMarkdown renders the Markdown twin for p.
func RenderMarkdown(p *Page) string {
	return p.RenderMarkdown()
}

// RenderJSON renders the structured JSON representation for p.
func RenderJSON(p *Page) ([]byte, error) {
	return p.RenderJSON()
}

// RenderHTMLFragment renders the semantic HTML fragment with <nav>, optional
// lead summary paragraph under <h1>, and <section id="..."> blocks.
func (p *Page) RenderHTMLFragment() string {
	var b strings.Builder
	b.WriteString(`<article data-path="` + html.EscapeString(p.Path) + `" data-revision="` + html.EscapeString(p.Revision) + `">` + "\n")
	if len(p.Nav) > 0 {
		b.WriteString(`<nav aria-label="Site">` + "\n")
		for _, n := range p.Nav {
			b.WriteString(`<a href="` + html.EscapeString(n.Href) + `">` + html.EscapeString(n.Label) + `</a>` + "\n")
		}
		b.WriteString("</nav>\n")
	}
	b.WriteString("<h1>" + html.EscapeString(p.Title) + "</h1>\n")
	if p.Summary != "" {
		b.WriteString("<p>" + renderInline(p.Summary) + "</p>\n")
	}
	for _, s := range p.Sections {
		b.WriteString(`<section id="` + html.EscapeString(s.ID) + `">` + "\n")
		b.WriteString("<h2>" + html.EscapeString(s.Heading) + "</h2>\n")
		if s.HTML != "" {
			b.WriteString(s.HTML + "\n")
		}
		b.WriteString("</section>\n")
	}
	b.WriteString("</article>\n")
	return b.String()
}

// RenderHTMLDocument renders a standalone HTML5 page carrying canonical,
// alternate (.md and .json), and revision metadata in <head>.
func (s *Site) RenderHTMLDocument(p *Page) string {
	mdTwin, jsonTwin := TwinPaths(p.Path)
	canonicalURL := s.Config.Origin + p.Path
	docTitle := p.Title
	if s.Config.Title != "" && p.Title != s.Config.Title {
		docTitle = p.Title + " — " + s.Config.Title
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>" + html.EscapeString(docTitle) + "</title>\n")
	b.WriteString(`<meta name="revision" content="` + html.EscapeString(p.Revision) + `">` + "\n")
	b.WriteString(`<link rel="canonical" href="` + html.EscapeString(canonicalURL) + `">` + "\n")
	b.WriteString(`<link rel="alternate" type="text/markdown" href="` + html.EscapeString(mdTwin) + `">` + "\n")
	b.WriteString(`<link rel="alternate" type="application/json" href="` + html.EscapeString(jsonTwin) + `">` + "\n")
	b.WriteString("<style>body{max-width:44rem;margin:2rem auto;padding:0 1rem;font:16px/1.6 system-ui,sans-serif;color:#111}nav{display:flex;gap:1rem;margin-bottom:2rem;border-bottom:1px solid #ddd;padding-bottom:.75rem}section{margin:1.75rem 0}code,pre{font-family:ui-monospace,monospace;background:#f5f5f5}code{padding:.1rem .3rem;border-radius:3px}pre{padding:.75rem;overflow-x:auto;border-radius:4px}</style>\n")
	b.WriteString("</head>\n<body>\n<main>\n")
	b.WriteString(p.RenderHTMLFragment())
	b.WriteString("</main>\n</body>\n</html>\n")
	return b.String()
}

// RenderMarkdown renders the Markdown twin with front matter (path, revision,
// alternates), labeled navigation links, optional lead summary paragraph under
// the title, and {#id} section headings.
func (p *Page) RenderMarkdown() string {
	mdTwin, jsonTwin := TwinPaths(p.Path)
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("path: " + yamlScalar(p.Path) + "\n")
	b.WriteString("title: " + yamlScalar(p.Title) + "\n")
	b.WriteString("revision: " + p.Revision + "\n")
	b.WriteString("alternates:\n")
	b.WriteString("  html: " + yamlScalar(p.Path) + "\n")
	b.WriteString("  markdown: " + yamlScalar(mdTwin) + "\n")
	b.WriteString("  json: " + yamlScalar(jsonTwin) + "\n")
	b.WriteString("---\n\n")
	if len(p.Nav) > 0 {
		parts := make([]string, 0, len(p.Nav))
		for _, n := range p.Nav {
			parts = append(parts, "["+n.Label+"]("+n.Href+")")
		}
		b.WriteString(strings.Join(parts, " · ") + "\n\n")
	}
	b.WriteString("# " + p.Title + "\n")
	if p.Summary != "" {
		b.WriteString("\n" + p.Summary + "\n")
	}
	for _, s := range p.Sections {
		b.WriteString("\n## " + s.Heading + " {#" + s.ID + "}\n")
		if s.Markdown != "" {
			b.WriteString("\n" + s.Markdown + "\n")
		}
	}
	return b.String()
}

// RenderJSON renders the structured JSON representation of the page.
func (p *Page) RenderJSON() ([]byte, error) {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// RenderRobotsTxt renders RFC 9309 /robots.txt.
func (s *Site) RenderRobotsTxt() string {
	return "User-agent: *\nAllow: /\n"
}

// RenderLLMsTxt renders /llms.txt listing all published pages and their twins.
func (s *Site) RenderLLMsTxt() string {
	var b strings.Builder
	b.WriteString("# " + s.Config.Title + "\n\n")
	b.WriteString("## Pages\n\n")
	for _, p := range s.Pages {
		mdTwin, jsonTwin := TwinPaths(p.Path)
		b.WriteString(fmt.Sprintf("- [%s](%s%s): markdown %s%s · json %s%s · revision %s\n",
			p.Title, s.Config.Origin, p.Path,
			s.Config.Origin, mdTwin,
			s.Config.Origin, jsonTwin,
			p.Revision))
	}
	return b.String()
}

func yamlScalar(s string) string {
	if strings.ContainsAny(s, ":#\"'\n\r\t{}[]") || strings.TrimSpace(s) != s {
		escaped := strings.ReplaceAll(s, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}

func renderMarkdownBlocks(md string) string {
	if strings.TrimSpace(md) == "" {
		return ""
	}
	lines := strings.Split(md, "\n")
	var blocks []string
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			i++
			continue
		}
		// Fenced code block.
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			ch := trimmed[0]
			n := 0
			for n < len(trimmed) && trimmed[n] == ch {
				n++
			}
			fence := trimmed[:n]
			lang := strings.TrimSpace(trimmed[n:])
			i++
			var code []string
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if strings.HasPrefix(t, fence) && strings.Trim(t, string(ch)) == "" {
					i++
					break
				}
				code = append(code, lines[i])
				i++
			}
			attr := ""
			if lang != "" {
				attr = ` class="language-` + html.EscapeString(lang) + `"`
			}
			blocks = append(blocks, "<pre><code"+attr+">"+html.EscapeString(strings.Join(code, "\n"))+"</code></pre>")
			continue
		}
		// Subheadings inside a section.
		if strings.HasPrefix(lines[i], "#### ") {
			blocks = append(blocks, "<h4>"+renderInline(strings.TrimSpace(lines[i][5:]))+"</h4>")
			i++
			continue
		}
		if strings.HasPrefix(lines[i], "### ") {
			blocks = append(blocks, "<h3>"+renderInline(strings.TrimSpace(lines[i][4:]))+"</h3>")
			i++
			continue
		}
		// Unordered list.
		if isUnorderedItem(trimmed) {
			var items []string
			for i < len(lines) && isUnorderedItem(strings.TrimSpace(lines[i])) {
				t := strings.TrimSpace(lines[i])
				items = append(items, "<li>"+renderInline(strings.TrimSpace(t[2:]))+"</li>")
				i++
			}
			blocks = append(blocks, "<ul>\n"+strings.Join(items, "\n")+"\n</ul>")
			continue
		}
		// Ordered list.
		if itemText, ok := cutOrderedItem(trimmed); ok {
			var items []string
			items = append(items, "<li>"+renderInline(itemText)+"</li>")
			i++
			for i < len(lines) {
				if nextText, ok := cutOrderedItem(strings.TrimSpace(lines[i])); ok {
					items = append(items, "<li>"+renderInline(nextText)+"</li>")
					i++
				} else {
					break
				}
			}
			blocks = append(blocks, "<ol>\n"+strings.Join(items, "\n")+"\n</ol>")
			continue
		}
		// Paragraph.
		var para []string
		for i < len(lines) {
			t := strings.TrimSpace(lines[i])
			if t == "" || strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") ||
				strings.HasPrefix(lines[i], "### ") || strings.HasPrefix(lines[i], "#### ") ||
				isUnorderedItem(t) {
				break
			}
			if _, ok := cutOrderedItem(t); ok {
				break
			}
			para = append(para, t)
			i++
		}
		blocks = append(blocks, "<p>"+renderInline(strings.Join(para, " "))+"</p>")
	}
	return strings.Join(blocks, "\n")
}

func isUnorderedItem(s string) bool {
	return strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ")
}

func cutOrderedItem(s string) (string, bool) {
	j := 0
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == 0 || j+1 >= len(s) || s[j] != '.' || s[j+1] != ' ' {
		return "", false
	}
	return strings.TrimSpace(s[j+2:]), true
}

func renderInline(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		// Inline code span `...`.
		if s[i] == '`' {
			end := strings.IndexByte(s[i+1:], '`')
			if end >= 0 {
				b.WriteString("<code>" + html.EscapeString(s[i+1:i+1+end]) + "</code>")
				i += end + 2
				continue
			}
		}
		// Markdown link [label](href).
		if s[i] == '[' {
			if label, href, consumed, ok := parseMarkdownLinkAt(s[i:]); ok {
				b.WriteString(`<a href="` + html.EscapeString(href) + `">` + renderInline(label) + `</a>`)
				i += consumed
				continue
			}
		}
		// Bold **...**.
		if strings.HasPrefix(s[i:], "**") {
			if end := strings.Index(s[i+2:], "**"); end > 0 {
				b.WriteString("<strong>" + renderInline(s[i+2:i+2+end]) + "</strong>")
				i += end + 4
				continue
			}
		}
		// Emphasis *...*.
		if s[i] == '*' {
			if end := strings.IndexByte(s[i+1:], '*'); end > 0 {
				b.WriteString("<em>" + renderInline(s[i+1:i+1+end]) + "</em>")
				i += end + 2
				continue
			}
		}
		b.WriteString(html.EscapeString(s[i : i+1]))
		i++
	}
	return b.String()
}

func parseMarkdownLinkAt(s string) (label, href string, consumed int, ok bool) {
	if len(s) == 0 || s[0] != '[' {
		return "", "", 0, false
	}
	depth := 0
	closeBracket := -1
	for i := 0; i < len(s); i++ {
		if s[i] == '`' {
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				i += end + 1
				continue
			}
		}
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				closeBracket = i
			}
		}
		if closeBracket >= 0 {
			break
		}
	}
	if closeBracket < 0 || closeBracket+1 >= len(s) || s[closeBracket+1] != '(' {
		return "", "", 0, false
	}
	parenDepth := 0
	closeParen := -1
	for i := closeBracket + 1; i < len(s); i++ {
		switch s[i] {
		case '(':
			parenDepth++
		case ')':
			parenDepth--
			if parenDepth == 0 {
				closeParen = i
			}
		}
		if closeParen >= 0 {
			break
		}
	}
	if closeParen < 0 {
		return "", "", 0, false
	}
	label = s[1:closeBracket]
	href = strings.TrimSpace(s[closeBracket+2 : closeParen])
	if href == "" {
		return "", "", 0, false
	}
	return label, href, closeParen + 1, true
}

var (
	htmlSectionRe = regexp.MustCompile(`<section\s+id="([^"]+)"`)
	htmlAnchorRe  = regexp.MustCompile(`<a\s+[^>]*href="([^"]+)"`)
)

// ExtractHTMLSectionIDs returns every <section id="..."> identifier in order.
func ExtractHTMLSectionIDs(doc string) []string {
	matches := htmlSectionRe.FindAllStringSubmatch(doc, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, html.UnescapeString(m[1]))
	}
	return out
}

// ExtractHTMLLinks returns every <a href="..."> target in order.
func ExtractHTMLLinks(doc string) []string {
	matches := htmlAnchorRe.FindAllStringSubmatch(doc, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, html.UnescapeString(m[1]))
	}
	return out
}

// ExtractMarkdownSectionIDs returns declared {#id} section ids from a rendered
// Markdown document in order.
func ExtractMarkdownSectionIDs(doc string) []string {
	body := stripFrontMatter(doc)
	var out []string
	var fence string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, string(fence[0])) == "" {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			ch := trimmed[0]
			n := 0
			for n < len(trimmed) && trimmed[n] == ch {
				n++
			}
			fence = trimmed[:n]
			continue
		}
		if strings.HasPrefix(line, "## ") {
			text := strings.TrimSpace(strings.TrimPrefix(line, "##"))
			if open := strings.LastIndex(text, "{#"); open >= 0 && strings.HasSuffix(text, "}") {
				out = append(out, strings.TrimSpace(text[open+2:len(text)-1]))
			}
		}
	}
	return out
}

// ExtractMarkdownLinks returns every Markdown link href outside front matter,
// fenced code blocks, and inline code spans, in order.
func ExtractMarkdownLinks(doc string) []string {
	body := stripFrontMatter(doc)
	var out []string
	var fence string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, string(fence[0])) == "" {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			ch := trimmed[0]
			n := 0
			for n < len(trimmed) && trimmed[n] == ch {
				n++
			}
			fence = trimmed[:n]
			continue
		}
		out = append(out, extractInlineMarkdownLinks(line)...)
	}
	return out
}

func extractInlineMarkdownLinks(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		if s[i] == '`' {
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				i += end + 2
				continue
			}
		}
		if s[i] == '[' {
			if _, href, consumed, ok := parseMarkdownLinkAt(s[i:]); ok {
				out = append(out, href)
				i += consumed
				continue
			}
		}
		i++
	}
	return out
}

func stripFrontMatter(doc string) string {
	norm := strings.ReplaceAll(doc, "\r\n", "\n")
	if !strings.HasPrefix(norm, "---\n") {
		return norm
	}
	end := strings.Index(norm[4:], "\n---")
	if end < 0 {
		return norm
	}
	return norm[4+end+4:]
}

// ExtractJSONSectionIDs decodes a rendered JSON page and returns section ids.
func ExtractJSONSectionIDs(raw []byte) ([]string, error) {
	var p Page
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(p.Sections))
	for _, s := range p.Sections {
		out = append(out, s.ID)
	}
	return out, nil
}

// ExtractJSONLinks decodes a rendered JSON page, verifies each section's HTML
// and Markdown carry identical links, and returns nav + summary + section links
// in order.
func ExtractJSONLinks(raw []byte) ([]string, error) {
	var p Page
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	var out []string
	for _, n := range p.Nav {
		out = append(out, n.Href)
	}
	if p.Summary != "" {
		out = append(out, extractInlineMarkdownLinks(p.Summary)...)
	}
	for _, s := range p.Sections {
		htmlLinks := ExtractHTMLLinks(s.HTML)
		mdLinks := ExtractMarkdownLinks(s.Markdown)
		if !slices.Equal(htmlLinks, mdLinks) {
			return nil, fmt.Errorf("section %q JSON html links %v != markdown links %v", s.ID, htmlLinks, mdLinks)
		}
		out = append(out, htmlLinks...)
	}
	return out, nil
}
