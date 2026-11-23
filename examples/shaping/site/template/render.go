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
	"sync"
)

// Assets holds the module's embedded site.json and content/*.md files so an
// external module importing this repository at a pinned version can render and
// serve its published content without a separate filesystem checkout.
//
//go:embed site.json content
var Assets embed.FS

// Config is site.json: site-wide metadata and shared navigation.
type Config struct {
	Title           string    `json:"title"`
	Origin          string    `json:"origin"`
	Owner           string    `json:"owner,omitempty"`
	Module          string    `json:"module,omitempty"`
	Source          string    `json:"source,omitempty"`
	EditURL         string    `json:"edit_url,omitempty"`
	MCP             string    `json:"mcp,omitempty"`
	Skill           string    `json:"skill,omitempty"`
	Shape           string    `json:"shape,omitempty"`
	AgentUserAgents []string  `json:"agent_user_agents,omitempty"`
	Nav             []NavItem `json:"nav"`
	// Verified bashy agents (botauth.go): Ed25519 keys trusted directly,
	// allowlisted key directory URLs a signed request may name, and the
	// public origins besides Origin (e.g. a front's mount URL) a signature
	// may be addressed to.
	TrustedBotKeys        []TrustedBotKey `json:"trusted_bot_keys,omitempty"`
	TrustedKeyDirectories []string        `json:"trusted_key_directories,omitempty"`
	TrustedOrigins        []string        `json:"trusted_origins,omitempty"`
}

var DefaultAgentUserAgents = []string{"Claude-User", "ClaudeCode", "claude-code", "Codex", "codex-cli", "Gemini-CLI", "gemini-cli", "OpenCode", "opencode", "ChatGPT-User", "OAI-SearchBot", "Perplexity-User", "Google-Extended", "bashy-fetch", "bashy-site"}

func (c *Config) applyDefaults() {
	c.Module = strings.TrimSpace(c.Module)
	c.Source = strings.TrimSpace(c.Source)
	c.EditURL = strings.TrimSpace(c.EditURL)
	c.MCP = strings.TrimSpace(c.MCP)
	c.Skill = strings.TrimSpace(c.Skill)
	c.Shape = strings.TrimSpace(c.Shape)
	if c.Module == "" {
		if c.Owner != "" {
			c.Module = "github.com/" + c.Owner + "/site"
		} else {
			c.Module = "site"
		}
	}
	if c.Source == "" && c.Owner != "" {
		c.Source = "https://github.com/" + c.Owner + "/site"
	}
	if c.EditURL == "" {
		c.EditURL = c.Source + "/edit/main/{source}"
	}
	if c.MCP == "" {
		c.MCP = c.Origin + "/mcp"
	}
	if c.Skill == "" {
		c.Skill = c.Origin + "/skill.md"
	}
	if c.Shape == "" {
		c.Shape = c.Origin + "/?shape=open"
	}
	if c.AgentUserAgents == nil {
		c.AgentUserAgents = append([]string{}, DefaultAgentUserAgents...)
	}
}

// AgentInfo is the agent block in structured form.
type AgentInfo struct {
	Instructions string `json:"instructions"`
	AgentsTxt    string `json:"agents_txt"`
	Index        string `json:"index"`
	Skills       string `json:"skills"`
	Skill        string `json:"skill"`
	MCP          string `json:"mcp"`
	Source       string `json:"source"`
	Edit         string `json:"edit"`
	Shape        string `json:"shape,omitempty"`
	Cite         string `json:"cite"`
	Recipe       string `json:"recipe,omitempty"`
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
	Agents       *AgentInfo `json:"agents,omitempty"`
	Path         string     `json:"path"`
	Title        string     `json:"title"`
	Summary      string     `json:"summary,omitempty"`
	Revision     string     `json:"revision"`
	Nav          []NavItem  `json:"nav"`
	Sections     []Section  `json:"sections"`
	Source       string     `json:"-"`
	DeclaredList []string   `json:"-"`
	site         *Site
}

// Site holds the loaded configuration and pages in deterministic order.
type Site struct {
	Config Config
	Pages  []*Page
	ByPath map[string]*Page

	botOnce sync.Once
	bot     *botVerifier
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
	cfg.applyDefaults()
	if err := validateBotTrust(&cfg); err != nil {
		return nil, err
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

	for _, p := range site.Pages {
		p.site = site
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
	old := s.Config.Origin
	newOrigin := strings.TrimRight(strings.TrimSpace(origin), "/")
	if s.Config.MCP == old+"/mcp" {
		s.Config.MCP = newOrigin + "/mcp"
	}
	if s.Config.Skill == old+"/skill.md" {
		s.Config.Skill = newOrigin + "/skill.md"
	}
	if s.Config.Shape == old+"/?shape=open" {
		s.Config.Shape = newOrigin + "/?shape=open"
	}
	s.Config.Origin = newOrigin
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

func (s *Site) homePage() *Page {
	if p, ok := s.ByPath["/"]; ok {
		return p
	}
	if len(s.Pages) > 0 {
		return s.Pages[0]
	}
	return nil
}
func (s *Site) SiteRevision() string {
	if p := s.homePage(); p != nil {
		return p.Revision
	}
	return ""
}
func (s *Site) SkillName() string {
	if s.Config.Owner != "" {
		return s.Config.Owner + "-site"
	}
	return "site"
}
func (s *Site) EditURL(p *Page) string {
	if p == nil {
		return strings.ReplaceAll(s.Config.EditURL, "{source}", "")
	}
	return strings.ReplaceAll(s.Config.EditURL, "{source}", p.Source)
}
func (s *Site) AgentInfoFor(p *Page) *AgentInfo {
	if p == nil {
		return nil
	}
	mdTwin, _ := TwinPaths(p.Path)
	_ = mdTwin
	return &AgentInfo{
		Instructions: s.Config.Origin + p.Path + "#agents",
		AgentsTxt:    s.Config.Origin + "/agents.txt",
		Index:        s.Config.Origin + "/llms.txt",
		Skills:       s.Config.Origin + "/.well-known/agent-skills/index.json",
		Skill:        s.Config.Skill,
		MCP:          s.Config.MCP,
		Source:       s.Config.Source,
		Edit:         s.EditURL(p),
		Shape:        s.Config.Shape,
		Cite:         s.Config.Origin + p.Path + " @ " + p.Revision + " {url, section_id, revision, section_sha256, quote}",
		Recipe:       s.AgentRecipe(p),
	}
}
func (p *Page) agentSite() *Site { return p.site }
func (s *Site) AgentRecipe(p *Page) string {
	mdTwin, jsonTwin := TwinPaths(p.Path)
	lines := []string{
		"# Agent recipe for " + s.Config.Title,
		"# Read /agents.txt first, then fetch the Markdown twin.",
		"# Next: bashy site attach " + s.Config.Origin + "  (or connect MCP at " + s.Config.MCP + ")",
		"# Skill: " + s.Config.Skill,
		"# JSON twin: " + s.Config.Origin + jsonTwin,
		"# Cite sections as {url, section_id, revision, section_sha256, quote}; revision " + p.Revision,
		"bashy fetch " + s.Config.Origin + "/agents.txt",
		"bashy fetch " + s.Config.Origin + mdTwin,
	}
	return strings.Join(lines, "\n")
}
func (s *Site) AgentBlockBody(p *Page) string {
	info := s.AgentInfoFor(p)
	mdTwin, jsonTwin := TwinPaths(p.Path)
	lines := []string{
		"This site is agent-ready and shaping-enabled, built with bashy. If you are an agent, start here.",
		"Read this page as Markdown at " + s.Config.Origin + mdTwin + " or as JSON at " + s.Config.Origin + jsonTwin + " ; use Accept: text/markdown for the Markdown twin. The site index is " + info.Index + " and the agent entry point is " + info.AgentsTxt + " .",
		"Equip yourself with the skill at " + info.Skill + " (skills index " + info.Skills + ") and connect to MCP at " + info.MCP + " . Run: bashy site attach " + s.Config.Origin,
		"Source: " + info.Source + " . Propose changes through the per-page edit URL " + info.Edit + " and pull requests.",
		"Shape: register the source as a shaped app, then wish, verify, preview, approve and undo. Shape URL: " + info.Shape,
		"Cite sections as {url, section_id, revision, section_sha256, quote}; this page revision is " + p.Revision + " .",
		"People reading over the agent shoulder: this block is the site instructions for agents; the human content follows the same source.",
	}
	return strings.Join(lines, "\n\n")
}
func (s *Site) AgentBlockMarkdown(p *Page) string {
	return "## For agents {#agents}\n\n" + s.AgentBlockBody(p) + "\n"
}
func (s *Site) RenderAgentsTxt() string {
	p := s.homePage()
	if p == nil {
		return ""
	}
	info := s.AgentInfoFor(p)
	mdTwin, jsonTwin := TwinPaths(p.Path)
	var b strings.Builder
	b.WriteString("site: " + s.Config.Origin + "\n")
	b.WriteString("title: " + s.Config.Title + "\n")
	b.WriteString("module: " + s.Config.Module + "\n")
	b.WriteString("revision: " + p.Revision + "\n")
	b.WriteString("index: " + info.Index + "\n")
	b.WriteString("agents_md: " + s.Config.Origin + "/AGENTS.md\n")
	b.WriteString("markdown: " + s.Config.Origin + mdTwin + "\n")
	b.WriteString("json: " + s.Config.Origin + jsonTwin + "\n")
	b.WriteString("skill: " + info.Skill + "\n")
	b.WriteString("skills_index: " + info.Skills + "\n")
	b.WriteString("mcp: " + info.MCP + "\n")
	b.WriteString("mcp_card: " + s.Config.Origin + "/.well-known/mcp/server-card.json\n")
	b.WriteString("source: " + info.Source + "\n")
	b.WriteString("edit: " + info.Edit + "\n")
	b.WriteString("shape: " + info.Shape + "\n")
	b.WriteString("cite: " + info.Cite + "\n")
	b.WriteString("bashy: bashy site attach " + s.Config.Origin + "\n")
	b.WriteString("\n")
	b.WriteString(s.AgentBlockMarkdown(p))
	return b.String()
}
func (s *Site) RenderAGENTSmd() string {
	p := s.homePage()
	var b strings.Builder
	b.WriteString("# AGENTS.md\n\n")
	if p != nil {
		b.WriteString(s.AgentBlockMarkdown(p) + "\n")
	}
	b.WriteString("## Pages\n\n")
	for _, pg := range s.Pages {
		mdTwin, jsonTwin := TwinPaths(pg.Path)
		b.WriteString("- " + pg.Title + ": " + s.Config.Origin + pg.Path + " markdown " + s.Config.Origin + mdTwin + " json " + s.Config.Origin + jsonTwin + " revision " + pg.Revision + "\n")
	}
	return b.String()
}
func (s *Site) RenderSkillMD() string {
	name := s.SkillName()
	return "---\nname: " + name + "\ndescription: Read and cite pages on " + s.Config.Title + " (" + s.Config.Origin + ") via /agents.txt, Markdown and JSON twins, and MCP.\nmetadata:\n  requires: \"has=bashy\"\n---\n# " + name + "\n\nOrigin: " + s.Config.Origin + "\n\nRead " + s.Config.Origin + "/agents.txt first, fetch the .md and .json twins, cite as {url, section_id, revision, section_sha256, quote}, and connect to MCP at " + s.Config.MCP + ".\n"
}
func (s *Site) RenderReadmeMD() string {
	return "# " + s.Config.Title + "\n\n" + s.Config.Origin + "\n\nAgent-ready site. Read /agents.txt and /AGENTS.md for agent instructions; see /llms.txt for the index.\n"
}
func (s *Site) RenderHumansTxt() string {
	return "/* SITE */\nSite: " + s.Config.Title + "\nOrigin: " + s.Config.Origin + "\n\n/* AGENTS */\nAgents: " + s.Config.Origin + "/agents.txt\nInstructions: " + s.Config.Origin + "/AGENTS.md\nMCP: " + s.Config.MCP + "\nSkill: " + s.Config.Skill + "\n"
}
func (s *Site) RenderAgentSkillsIndex() string {
	skill := s.RenderSkillMD()
	sum := sha256.Sum256([]byte(skill))
	obj := map[string]any{"version": "0.2.0", "skills": []map[string]any{{"name": s.SkillName(), "type": "skill-md", "description": "Read and cite pages on " + s.Config.Title, "url": "/skill.md", "sha256": hex.EncodeToString(sum[:])}}}
	b, _ := json.MarshalIndent(obj, "", "  ")
	return string(b) + "\n"
}
func (s *Site) RenderMCPServerCard() string {
	obj := map[string]any{"schema_version": "mcp-server-card-v1", "name": s.SkillName(), "title": s.Config.Title, "version": "0.2.0", "endpoint": s.Config.MCP, "transport": "streamable-http", "authentication": map[string]any{"type": "cookie-or-oauth2", "oauth_protected_resource": s.Config.Origin + "/.well-known/oauth-protected-resource"}, "tools": []string{"list_pages", "read_page", "cite", "shape_wish", "shape_verify", "shape_preview", "shape_approve", "shape_undo"}}
	b, _ := json.MarshalIndent(obj, "", "  ")
	return string(b) + "\n"
}
func (s *Site) RenderOAuthProtectedResource() string {
	obj := map[string]any{"resource": s.Config.MCP, "authorization_servers": []string{s.Config.Origin}}
	b, _ := json.MarshalIndent(obj, "", "  ")
	return string(b) + "\n"
}
func (s *Site) RenderAgentCard() string {
	obj := map[string]any{"name": s.SkillName(), "description": "Read and cite pages on " + s.Config.Title, "url": s.Config.Origin, "version": "0.2.0", "capabilities": map[string]any{"streaming": false}, "skills": []map[string]any{{"id": s.SkillName(), "name": s.SkillName(), "description": "Read and cite pages on " + s.Config.Title}}}
	b, _ := json.MarshalIndent(obj, "", "  ")
	return string(b) + "\n"
}
func (s *Site) RenderAPICatalog() string {
	type link struct {
		Href string `json:"href"`
		Rel  string `json:"rel"`
		Type string `json:"type,omitempty"`
	}
	links := []link{{s.Config.MCP, "service-desc", "application/json"}, {s.Config.Origin + "/agents.txt", "service-doc", "text/plain"}, {s.Config.Origin + "/llms.txt", "describedby", "text/plain"}}
	for _, p := range s.Pages {
		mdTwin, jsonTwin := TwinPaths(p.Path)
		links = append(links, link{s.Config.Origin + mdTwin, "alternate", "text/markdown"}, link{s.Config.Origin + jsonTwin, "alternate", "application/json"})
	}
	obj := map[string]any{"linkset": []map[string]any{{"anchor": s.Config.Origin, "items": links}}}
	b, _ := json.MarshalIndent(obj, "", "  ")
	return string(b) + "\n"
}
func (s *Site) RenderSecurityTxt() string {
	return "Policy: " + s.Config.Origin + "/AGENTS.md\nExpires: 2030-01-01T00:00:00.000Z\n"
}
func (s *Site) RenderSitemapXML() string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	for _, p := range s.Pages {
		mdTwin, jsonTwin := TwinPaths(p.Path)
		for _, u := range []string{s.Config.Origin + p.Path, s.Config.Origin + mdTwin, s.Config.Origin + jsonTwin} {
			b.WriteString("<url><loc>" + html.EscapeString(u) + "</loc></url>\n")
		}
	}
	b.WriteString("</urlset>\n")
	return b.String()
}
func (s *Site) RenderAtomFeed() string {
	p := s.homePage()
	desc := ""
	if p != nil {
		desc = s.AgentBlockMarkdown(p)
	}
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<feed xmlns=\"http://www.w3.org/2005/Atom\">\n<title>" + html.EscapeString(s.Config.Title) + "</title>\n<id>" + html.EscapeString(s.Config.Origin+"/") + "</id>\n<subtitle>" + html.EscapeString(desc) + "</subtitle>\n")
	for _, pg := range s.Pages {
		b.WriteString("<entry><title>" + html.EscapeString(pg.Title) + "</title><id>" + html.EscapeString(s.Config.Origin+pg.Path) + "</id><link href=\"" + html.EscapeString(s.Config.Origin+pg.Path) + "\"/></entry>\n")
	}
	b.WriteString("</feed>\n")
	return b.String()
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
	if p.site != nil {
		info := p.site.AgentInfoFor(p)
		b.WriteString(`<p id="agent-note" role="note">This site is agent-ready: instructions at #agents - MCP ` + html.EscapeString(info.MCP) + ` - skill ` + html.EscapeString(info.Skill) + `</p>` + "\n")
	}
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
	if p.site != nil {
		b.WriteString(`<section id="agents">` + "\n<h2>For agents</h2>\n")
		b.WriteString(renderMarkdownBlocks(p.site.AgentBlockBody(p)) + "\n")
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
	if p.site != nil {
		info := s.AgentInfoFor(p)
		b.WriteString(`<meta name="agent:instructions" content="` + html.EscapeString(info.Instructions) + `">` + "\n")
	}
	b.WriteString(`<meta name="generator" content="bashy site 0.2.0">` + "\n")
	b.WriteString(`<link rel="canonical" href="` + html.EscapeString(canonicalURL) + `">` + "\n")
	b.WriteString(`<link rel="alternate" type="text/markdown" href="` + html.EscapeString(mdTwin) + `">` + "\n")
	b.WriteString(`<link rel="alternate" type="application/json" href="` + html.EscapeString(jsonTwin) + `">` + "\n")
	b.WriteString(`<link rel="alternate" type="application/atom+xml" href="/feed.xml">` + "\n")
	b.WriteString(`<link rel="describedby" href="/llms.txt">` + "\n")
	if p.site != nil {
		b.WriteString(`<link rel="edit" href="` + html.EscapeString(s.EditURL(p)) + `">` + "\n")
		if s.Config.Source != "" {
			b.WriteString(`<link rel="via" href="` + html.EscapeString(s.Config.Source) + `">` + "\n")
		}
	}
	b.WriteString(`<link rel="service-doc" href="/AGENTS.md">` + "\n")
	b.WriteString(`<link rel="service-desc" href="/.well-known/mcp/server-card.json">` + "\n")
	b.WriteString(`<link rel="author" href="/humans.txt">` + "\n")
	b.WriteString(`<link rel="help" href="/llms.txt">` + "\n")
	// JSON-LD
	ld, _ := json.Marshal(map[string]any{"@context": "https://schema.org", "@graph": []any{map[string]any{"@type": "WebSite", "url": s.Config.Origin, "name": s.Config.Title}, map[string]any{"@type": "SoftwareSourceCode", "name": s.Config.Title, "codeRepository": s.Config.Source}}})
	b.WriteString(`<script type="application/ld+json">` + string(ld) + "</script>\n")
	b.WriteString("<style>body{max-width:44rem;margin:2rem auto;padding:0 1rem;font:16px/1.6 system-ui,sans-serif;color:#111}nav{display:flex;gap:1rem;margin-bottom:2rem;border-bottom:1px solid #ddd;padding-bottom:.75rem}section{margin:1.75rem 0}code,pre{font-family:ui-monospace,monospace;background:#f5f5f5}code{padding:.1rem .3rem;border-radius:3px}pre{padding:.75rem;overflow-x:auto;border-radius:4px}</style>\n")
	b.WriteString("</head>\n<body>\n")
	if p.site != nil {
		b.WriteString(`<script id="agent-recipe" type="text/bashsharp">` + "\n" + s.AgentRecipe(p) + "\n</script>\n")
		b.WriteString(`<div hidden>` + "\n<h2>For agents</h2>\n" + renderMarkdownBlocks(s.AgentBlockBody(p)) + "\n</div>\n")
		b.WriteString(`<noscript>` + "\n<h2>For agents</h2>\n" + renderMarkdownBlocks(s.AgentBlockBody(p)) + "\n</noscript>\n")
	}
	b.WriteString("<main>\n")
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
	if p.site != nil {
		info := p.site.AgentInfoFor(p)
		b.WriteString("agent:\n")
		b.WriteString("  instructions: " + yamlScalar(info.Instructions) + "\n")
		b.WriteString("  agents_txt: " + yamlScalar(info.AgentsTxt) + "\n")
		b.WriteString("  index: " + yamlScalar(info.Index) + "\n")
		b.WriteString("  skills: " + yamlScalar(info.Skills) + "\n")
		b.WriteString("  skill: " + yamlScalar(info.Skill) + "\n")
		b.WriteString("  mcp: " + yamlScalar(info.MCP) + "\n")
		b.WriteString("  source: " + yamlScalar(info.Source) + "\n")
		b.WriteString("  edit: " + yamlScalar(info.Edit) + "\n")
		b.WriteString("  cite: " + yamlScalar(info.Cite) + "\n")
	}
	b.WriteString("---\n\n")
	if p.site != nil {
		b.WriteString(p.site.AgentBlockMarkdown(p) + "\n")
	}
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
	cp := *p
	if p.site != nil {
		cp.Agents = p.site.AgentInfoFor(p)
	}
	b, err := json.MarshalIndent(&cp, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// RenderRobotsTxt renders RFC 9309 /robots.txt.
func (s *Site) RenderRobotsTxt() string {
	var b strings.Builder
	b.WriteString("# Agent-ready site: read /agents.txt and /AGENTS.md for agent instructions.\n")
	b.WriteString("# MCP: " + s.Config.MCP + " Skill: " + s.Config.Skill + "\n")
	b.WriteString("User-agent: *\nAllow: /\n")
	b.WriteString("Sitemap: " + s.Config.Origin + "/sitemap.xml\n")
	return b.String()
}

// RenderLLMsTxt renders /llms.txt listing all published pages and their twins.
func (s *Site) RenderLLMsTxt() string {
	var b strings.Builder
	b.WriteString("# " + s.Config.Title + "\n\n")
	if p := s.homePage(); p != nil {
		b.WriteString(s.AgentBlockMarkdown(p) + "\n")
	}
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
	out := make([]string, 0, len(p.Sections)+1)
	if p.Agents != nil {
		out = append(out, "agents")
	}
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
