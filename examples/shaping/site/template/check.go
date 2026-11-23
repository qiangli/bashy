package site

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
)

// CheckOutcome reports one named check result.
type CheckOutcome struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Validate enforces unique-ids, links-resolve, and representation parity on an
// already-loaded Site.
func Validate(s *Site) error {
	if err := checkUniqueIDsSite(s); err != nil {
		return err
	}
	if err := checkLinksResolveSite(s); err != nil {
		return err
	}
	return checkParitySite(s)
}

// ValidateSite is an alias for Validate.
func ValidateSite(s *Site) error {
	return Validate(s)
}

// RunChecks executes one check ("render-determinism", "unique-ids",
// "links-resolve", "stale-base") or "all".
func RunChecks(dir, baseDir, name, pageHint, element string) ([]CheckOutcome, error) {
	type step struct {
		name string
		fn   func() error
	}
	all := []step{
		{"render-determinism", func() error { return CheckRenderDeterminism(dir) }},
		{"unique-ids", func() error { return CheckUniqueIDs(dir) }},
		{"links-resolve", func() error { return CheckLinksResolve(dir) }},
		{"stale-base", func() error { return CheckStaleBase(baseDir, pageHint, element) }},
	}
	var selected []step
	if name == "all" {
		selected = all
	} else {
		for _, st := range all {
			if st.name == name {
				selected = append(selected, st)
				break
			}
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("unknown check %q", name)
		}
	}

	var outcomes []CheckOutcome
	var firstErr error
	for _, st := range selected {
		if err := st.fn(); err != nil {
			outcomes = append(outcomes, CheckOutcome{Name: st.name, Status: "fail", Detail: err.Error()})
			if firstErr == nil {
				firstErr = err
			}
		} else {
			outcomes = append(outcomes, CheckOutcome{Name: st.name, Status: "pass"})
		}
	}
	return outcomes, firstErr
}

// CheckRenderDeterminism loads and renders the site twice and verifies
// byte-identical outputs plus HTML/Markdown/JSON link and section-id parity.
func CheckRenderDeterminism(dir string) error {
	first, err := Load(dir)
	if err != nil {
		return err
	}
	second, err := Load(dir)
	if err != nil {
		return err
	}
	if len(first.Pages) != len(second.Pages) {
		return fmt.Errorf("render-determinism: page count %d != %d", len(first.Pages), len(second.Pages))
	}
	if first.RenderAgentsTxt() != second.RenderAgentsTxt() {
		return errors.New("render-determinism: /agents.txt differed across two renders")
	}
	if first.RenderRobotsTxt() != second.RenderRobotsTxt() {
		return errors.New("render-determinism: /robots.txt differed across two renders")
	}
	if first.RenderLLMsTxt() != second.RenderLLMsTxt() {
		return errors.New("render-determinism: /llms.txt differed across two renders")
	}
	for i, p1 := range first.Pages {
		p2 := second.Pages[i]
		if p1.Revision != p2.Revision {
			return fmt.Errorf("render-determinism: %s revision %s != %s", p1.Path, p1.Revision, p2.Revision)
		}
		if p1.RenderHTMLFragment() != p2.RenderHTMLFragment() {
			return fmt.Errorf("render-determinism: %s HTML fragment differed across two renders", p1.Path)
		}
		if first.RenderHTMLDocument(p1) != second.RenderHTMLDocument(p2) {
			return fmt.Errorf("render-determinism: %s HTML document differed across two renders", p1.Path)
		}
		if p1.RenderMarkdown() != p2.RenderMarkdown() {
			return fmt.Errorf("render-determinism: %s Markdown differed across two renders", p1.Path)
		}
		j1, err := p1.RenderJSON()
		if err != nil {
			return err
		}
		j2, err := p2.RenderJSON()
		if err != nil {
			return err
		}
		if !bytes.Equal(j1, j2) {
			return fmt.Errorf("render-determinism: %s JSON differed across two renders", p1.Path)
		}
	}
	return checkParitySite(first)
}

func checkParitySite(s *Site) error {
	for _, p := range s.Pages {
		htmlFrag := p.RenderHTMLFragment()
		htmlDoc := s.RenderHTMLDocument(p)
		mdDoc := p.RenderMarkdown()
		jsonDoc, err := p.RenderJSON()
		if err != nil {
			return err
		}

		idsFrag := ExtractHTMLSectionIDs(htmlFrag)
		idsDoc := ExtractHTMLSectionIDs(htmlDoc)
		idsMD := ExtractMarkdownSectionIDs(mdDoc)
		idsJSON, err := ExtractJSONSectionIDs(jsonDoc)
		if err != nil {
			return err
		}
		if !equalSorted(idsFrag, idsDoc) || !equalSorted(idsDoc, idsMD) || !equalSorted(idsDoc, idsJSON) {
			return fmt.Errorf("parity: %s section ids differ across representations: html=%v md=%v json=%v", p.Path, idsDoc, idsMD, idsJSON)
		}

		linksFrag := ExtractHTMLLinks(htmlFrag)
		linksDoc := ExtractHTMLLinks(htmlDoc)
		linksMD := ExtractMarkdownLinks(mdDoc)
		linksJSON, err := ExtractJSONLinks(jsonDoc)
		if err != nil {
			return fmt.Errorf("parity: %s: %w", p.Path, err)
		}
		if !equalSorted(linksFrag, linksDoc) || !equalSorted(linksDoc, linksMD) || !equalSorted(linksDoc, linksJSON) {
			return fmt.Errorf("parity: %s links differ across representations: html=%v md=%v json=%v", p.Path, linksDoc, linksMD, linksJSON)
		}
		if err := checkAgentPage(s, p, htmlDoc, mdDoc, jsonDoc); err != nil {
			return err
		}
	}
	// agents.txt equivalence with home block
	if home := s.homePage(); home != nil {
		at := s.RenderAgentsTxt()
		head, block, found := strings.Cut(at, "\n\n")
		if !found {
			return errors.New("agents: /agents.txt missing blank-line separator")
		}
		for _, k := range []string{"site:", "title:", "module:", "revision:", "index:", "agents_md:", "markdown:", "json:", "skill:", "skills_index:", "mcp:", "mcp_card:", "source:", "edit:", "shape:", "cite:", "bashy:"} {
			if !strings.Contains(head, k) {
				return fmt.Errorf("agents: /agents.txt missing key %s", k)
			}
		}
		if strings.TrimSpace(block) != strings.TrimSpace(s.AgentBlockMarkdown(home)) {
			return errors.New("agents: /agents.txt block differs from home agent block")
		}
		// well-known shapes
		var skills struct {
			Version string `json:"version"`
			Skills  []struct {
				Name string `json:"name"`
				URL  string `json:"url"`
				SHA  string `json:"sha256"`
			} `json:"skills"`
		}
		if err := json.Unmarshal([]byte(s.RenderAgentSkillsIndex()), &skills); err != nil || skills.Version != "0.2.0" || len(skills.Skills) != 1 || skills.Skills[0].URL != "/skill.md" || len(skills.Skills[0].SHA) != 64 {
			return fmt.Errorf("agents: invalid agent-skills index: %v", err)
		}
		var card map[string]any
		if err := json.Unmarshal([]byte(s.RenderMCPServerCard()), &card); err != nil || card["schema_version"] != "mcp-server-card-v1" || card["endpoint"] != s.Config.MCP {
			return fmt.Errorf("agents: invalid MCP server card: %v", err)
		}
		var oauth map[string]any
		if err := json.Unmarshal([]byte(s.RenderOAuthProtectedResource()), &oauth); err != nil || oauth["resource"] != s.Config.MCP {
			return fmt.Errorf("agents: invalid oauth-protected-resource: %v", err)
		}
	}
	return nil
}

func equalSorted(a, b []string) bool {
	ac := append([]string{}, a...)
	bc := append([]string{}, b...)
	slices.Sort(ac)
	slices.Sort(bc)
	return slices.Equal(ac, bc)
}

var tagStripRe = regexp.MustCompile(`<[^>]+>`)

func normalizeText(s string) string {
	s = tagStripRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return strings.Join(strings.Fields(s), " ")
}

func extractBetween(doc, start, end string) string {
	i := strings.Index(doc, start)
	if i < 0 {
		return ""
	}
	rest := doc[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func checkAgentPage(s *Site, p *Page, htmlDoc, mdDoc string, jsonDoc []byte) error {
	// JSON agents first field
	trimmed := strings.TrimSpace(string(jsonDoc))
	if !strings.HasPrefix(trimmed, "{\n  \"agents\":") && !strings.HasPrefix(trimmed, "{\"agents\":") {
		return fmt.Errorf("agents: %s JSON agents is not the first field", p.Path)
	}
	var decoded struct {
		Agents   *AgentInfo `json:"agents"`
		Revision string     `json:"revision"`
	}
	if err := json.Unmarshal(jsonDoc, &decoded); err != nil {
		return fmt.Errorf("agents: %s JSON: %w", p.Path, err)
	}
	if decoded.Agents == nil {
		return fmt.Errorf("agents: %s JSON missing agents", p.Path)
	}
	if decoded.Revision != p.Revision {
		return fmt.Errorf("agents: %s JSON revision %s != %s", p.Path, decoded.Revision, p.Revision)
	}
	want := s.AgentInfoFor(p)
	if *decoded.Agents != *want {
		return fmt.Errorf("agents: %s JSON agents %+v != %+v", p.Path, decoded.Agents, want)
	}
	// Markdown: block first after front matter, agent: front matter
	body := stripFrontMatter(mdDoc)
	if !strings.HasPrefix(strings.TrimSpace(body), "## For agents {#agents}") {
		return fmt.Errorf("agents: %s Markdown agent block is not first after front matter", p.Path)
	}
	for _, frag := range []string{"agent:\n", "  instructions:", "  agents_txt:", "  index:", "  skills:", "  skill:", "  mcp:", "  source:", "  edit:", "  cite:", "revision: " + p.Revision} {
		if !strings.Contains(mdDoc, frag) {
			return fmt.Errorf("agents: %s Markdown missing %q", p.Path, frag)
		}
	}
	if !strings.Contains(mdDoc, s.AgentBlockMarkdown(p)) {
		return fmt.Errorf("agents: %s Markdown block differs", p.Path)
	}
	// HTML head/link/meta
	for _, frag := range []string{
		`<meta name="agent:instructions" content="` + want.Instructions + `">`,
		`<meta name="generator" content="bashy site 0.2.0">`,
		`<meta name="revision" content="` + p.Revision + `">`,
		`<link rel="alternate" type="text/markdown"`,
		`<link rel="alternate" type="application/json"`,
		`<link rel="describedby" href="/llms.txt">`,
		`<link rel="edit" href="` + want.Edit + `">`,
		`<link rel="service-doc" href="/AGENTS.md">`,
		`<link rel="service-desc" href="/.well-known/mcp/server-card.json">`,
		`<link rel="canonical"`,
		`<link rel="author"`,
		`<link rel="help"`,
		`<script id="agent-recipe" type="text/bashsharp">`,
		`<p id="agent-note" role="note">`,
		`<section id="agents">`,
	} {
		if !strings.Contains(htmlDoc, frag) {
			return fmt.Errorf("agents: %s HTML missing %q", p.Path, frag)
		}
	}
	if !strings.Contains(htmlDoc, "WebSite") || !strings.Contains(htmlDoc, "SoftwareSourceCode") || !strings.Contains(htmlDoc, "codeRepository") || (s.Config.Source != "" && !strings.Contains(htmlDoc, s.Config.Source)) {
		return fmt.Errorf("agents: %s JSON-LD missing WebSite/SoftwareSourceCode codeRepository", p.Path)
	}
	if idxNote := strings.Index(htmlDoc, `id="agent-note"`); idxNote < 0 || (strings.Index(htmlDoc, "<nav") >= 0 && idxNote > strings.Index(htmlDoc, "<nav")) {
		return fmt.Errorf("agents: %s agent-note is not before nav", p.Path)
	}
	// script recipe: ASCII, no literal closing tag inside
	recipe := extractBetween(htmlDoc, `<script id="agent-recipe" type="text/bashsharp">`, "</script>")
	if recipe == "" {
		return fmt.Errorf("agents: %s missing bashsharp recipe", p.Path)
	}
	for _, r := range recipe {
		if r > 127 {
			return fmt.Errorf("agents: %s recipe is not plain ASCII", p.Path)
		}
	}
	if strings.Contains(recipe, "</script>") {
		return fmt.Errorf("agents: %s recipe contains closing script tag", p.Path)
	}
	// visible == hidden == noscript normalized
	visible := extractBetween(htmlDoc, `<section id="agents">`, "</section>")
	hidden := extractBetween(htmlDoc, `<div hidden>`, "</div>")
	noscript := extractBetween(htmlDoc, `<noscript>`, "</noscript>")
	if visible == "" || hidden == "" || noscript == "" {
		return fmt.Errorf("agents: %s missing visible/hidden/noscript block", p.Path)
	}
	if normalizeText(visible) != normalizeText(hidden) || normalizeText(visible) != normalizeText(noscript) {
		return fmt.Errorf("agents: %s hidden/noscript text differs from visible agents block", p.Path)
	}
	return nil
}

// CheckUniqueIDs verifies unique page paths and unique declared section ids.
func CheckUniqueIDs(dir string) error {
	site, err := Load(dir)
	if err != nil {
		return err
	}
	return checkUniqueIDsSite(site)
}

func checkUniqueIDsSite(s *Site) error {
	seenPaths := map[string]string{}
	for _, p := range s.Pages {
		if prev, dup := seenPaths[p.Path]; dup {
			return fmt.Errorf("unique-ids: duplicate page path %q in %s and %s", p.Path, prev, p.Source)
		}
		seenPaths[p.Path] = p.Source

		seenIDs := map[string]bool{}
		for _, sec := range p.Sections {
			if !sectionIDPattern.MatchString(sec.ID) {
				return fmt.Errorf("unique-ids: invalid section id %q on %s", sec.ID, p.Path)
			}
			if seenIDs[sec.ID] {
				return fmt.Errorf("unique-ids: duplicate section id %q on %s", sec.ID, p.Path)
			}
			seenIDs[sec.ID] = true
		}
	}
	return nil
}

// CheckLinksResolve verifies that every internal link in navigation, optional
// summary, and page sections resolves to a served route and (when #fragment is
// present) to a declared section id on the target page.
func CheckLinksResolve(dir string) error {
	site, err := Load(dir)
	if err != nil {
		return err
	}
	return checkLinksResolveSite(site)
}

func checkLinksResolveSite(s *Site) error {
	staticRoutes := map[string]bool{
		"/robots.txt": true, "/llms.txt": true, "/agents.txt": true, "/AGENTS.md": true, "/README.md": true, "/skill.md": true, "/humans.txt": true, "/sitemap.xml": true, "/feed.xml": true,
		"/.well-known/agent-skills/index.json": true, "/.well-known/mcp/server-card.json": true, "/.well-known/oauth-protected-resource": true, "/.well-known/agent-card.json": true, "/.well-known/api-catalog": true, "/.well-known/security.txt": true,
	}
	for _, p := range s.Pages {
		mdTwin, jsonTwin := TwinPaths(p.Path)
		staticRoutes[mdTwin] = true
		staticRoutes[jsonTwin] = true
	}

	checkHref := func(fromPath, href string) error {
		if strings.HasPrefix(href, "//") {
			return nil
		}
		u, err := url.Parse(href)
		if err != nil {
			return fmt.Errorf("links-resolve: invalid link %q on %s: %w", href, fromPath, err)
		}
		if u.Scheme != "" || u.Host != "" {
			return nil
		}
		targetPath := u.Path
		switch {
		case targetPath == "":
			targetPath = fromPath
		case !strings.HasPrefix(targetPath, "/"):
			baseDir := fromPath
			if !strings.HasSuffix(baseDir, "/") {
				baseDir = path.Dir(baseDir)
			}
			targetPath = path.Clean(path.Join(baseDir, targetPath))
		default:
			if targetPath != "/" {
				targetPath = path.Clean(targetPath)
			}
		}

		if targetPage, ok := s.ByPath[targetPath]; ok {
			if u.Fragment != "" {
				found := false
				for _, sec := range targetPage.Sections {
					if sec.ID == u.Fragment {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("links-resolve: link %q on %s points to missing section #%s on %s", href, fromPath, u.Fragment, targetPath)
				}
			}
			return nil
		}
		if staticRoutes[targetPath] {
			if u.Fragment != "" {
				return fmt.Errorf("links-resolve: link %q on %s specifies fragment #%s on non-HTML route %s", href, fromPath, u.Fragment, targetPath)
			}
			return nil
		}
		return fmt.Errorf("links-resolve: broken internal link %q on %s (target %s does not exist)", href, fromPath, targetPath)
	}

	for _, n := range s.Config.Nav {
		if err := checkHref("/", n.Href); err != nil {
			return err
		}
	}
	for _, p := range s.Pages {
		for _, href := range ExtractMarkdownLinks(p.RenderMarkdown()) {
			if err := checkHref(p.Path, href); err != nil {
				return err
			}
		}
	}
	return nil
}

// CheckStaleBase verifies compare-and-set on a wish's --element provenance
// ('section=ID revision=REV'): REV must match the base page's revision, else
// it fails with "stale, re-read". When element is empty, no provenance was
// supplied and the check passes.
func CheckStaleBase(baseDir, pageHint, element string) error {
	element = strings.TrimSpace(element)
	if element == "" {
		return nil
	}
	secID, wantRev, hasDirective, err := parseWishElement(element)
	if err != nil {
		return err
	}
	if !hasDirective {
		return nil
	}
	baseSite, err := Load(baseDir)
	if err != nil {
		return fmt.Errorf("stale, re-read: load base site: %w", err)
	}

	var target *Page
	pageHint = strings.TrimSpace(pageHint)
	if pageHint != "" {
		if pageHint != "/" && strings.HasPrefix(pageHint, "/") {
			pageHint = path.Clean(pageHint)
		}
		p, ok := baseSite.ByPath[pageHint]
		if !ok {
			return fmt.Errorf("stale, re-read: base site has no page %q", pageHint)
		}
		target = p
	} else {
		var matches []*Page
		for _, p := range baseSite.Pages {
			for _, s := range p.Sections {
				if s.ID == secID {
					matches = append(matches, p)
					break
				}
			}
		}
		if len(matches) == 0 {
			return fmt.Errorf("stale, re-read: base site has no section %q", secID)
		}
		if len(matches) > 1 {
			return fmt.Errorf("stale, re-read: section %q appears on multiple pages; pass --page", secID)
		}
		target = matches[0]
	}

	hasSection := false
	for _, s := range target.Sections {
		if s.ID == secID {
			hasSection = true
			break
		}
	}
	if !hasSection {
		return fmt.Errorf("stale, re-read: page %s has no section %q (base revision %s)", target.Path, secID, target.Revision)
	}
	if target.Revision != wantRev {
		return fmt.Errorf("stale, re-read: page %s section %s is at revision %s, wish had %s", target.Path, secID, target.Revision, wantRev)
	}
	return nil
}

func parseWishElement(element string) (sectionID, revision string, hasDirective bool, err error) {
	for _, tok := range strings.Fields(element) {
		tok = strings.Trim(tok, ",;\"'")
		k, v, ok := strings.Cut(tok, "=")
		if !ok {
			continue
		}
		switch k {
		case "section":
			sectionID = strings.TrimPrefix(v, "#")
			hasDirective = true
		case "revision":
			revision = v
			hasDirective = true
		}
	}
	if !hasDirective {
		return "", "", false, nil
	}
	if sectionID == "" || revision == "" {
		return "", "", true, fmt.Errorf("stale, re-read: wish element %q must carry both section=ID and revision=REV", element)
	}
	return sectionID, revision, true, nil
}

var inlineDirectiveRe = regexp.MustCompile(`\bsection=([#a-zA-Z0-9_-]+)\s+revision=([a-zA-Z0-9_-]+)|\brevision=([a-zA-Z0-9_-]+)\s+section=([#a-zA-Z0-9_-]+)`)

// ResolveWishInput extracts the wish page and element provenance from explicit
// flags, an optional .wish / wish.json file, or a git commit message file.
func ResolveWishInput(pageHint, element, wishFile, commitMsgFile string) (string, string, error) {
	if strings.TrimSpace(element) != "" {
		return pageHint, element, nil
	}
	if wishFile != "" {
		if raw, err := os.ReadFile(wishFile); err == nil {
			p, e, perr := parseWishFile(raw)
			if perr != nil {
				return "", "", perr
			}
			if e != "" {
				if pageHint == "" {
					pageHint = p
				}
				return pageHint, e, nil
			}
		}
	}
	if commitMsgFile != "" {
		if raw, err := os.ReadFile(commitMsgFile); err == nil {
			p, e := parseCommitWish(string(raw))
			if e != "" {
				if pageHint == "" {
					pageHint = p
				}
				return pageHint, e, nil
			}
		}
	}
	return pageHint, "", nil
}

func parseWishFile(raw []byte) (pageHint, element string, err error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", "", nil
	}
	if strings.HasPrefix(trimmed, "{") {
		var w struct {
			Page    string `json:"page"`
			Element string `json:"element"`
		}
		if jerr := json.Unmarshal([]byte(trimmed), &w); jerr != nil {
			return "", "", fmt.Errorf("stale, re-read: invalid wish JSON: %w", jerr)
		}
		return strings.TrimSpace(w.Page), strings.TrimSpace(w.Element), nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "section=") || strings.Contains(line, "revision=") {
			if strings.HasPrefix(line, "element:") || strings.HasPrefix(line, "element=") {
				_, v, _ := strings.Cut(line, line[7:8])
				element = unquoteYAML(strings.TrimSpace(v))
				continue
			}
			if !strings.HasPrefix(line, "request:") && !strings.HasPrefix(line, "request=") {
				element = line
				continue
			}
		}
		for _, sep := range []string{":", "="} {
			if k, v, ok := strings.Cut(line, sep); ok {
				k = strings.ToLower(strings.TrimSpace(k))
				v = unquoteYAML(strings.TrimSpace(v))
				switch k {
				case "page":
					pageHint = v
				case "element":
					element = v
				}
				break
			}
		}
	}
	return pageHint, element, nil
}

func parseCommitWish(msg string) (pageHint, element string) {
	for _, line := range strings.Split(strings.ReplaceAll(msg, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if k, v, ok := strings.Cut(trimmed, ":"); ok {
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "shape-page":
				pageHint = strings.TrimSpace(v)
			case "shape-element":
				element = strings.TrimSpace(v)
			}
		}
	}
	if element != "" {
		return pageHint, element
	}
	if m := inlineDirectiveRe.FindStringSubmatch(msg); m != nil {
		if m[1] != "" {
			return pageHint, "section=" + m[1] + " revision=" + m[2]
		}
		return pageHint, "section=" + m[4] + " revision=" + m[3]
	}
	return pageHint, ""
}
