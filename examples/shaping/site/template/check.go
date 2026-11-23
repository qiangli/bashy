package main

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

// ValidateSite enforces unique-ids, links-resolve, and representation parity
// on an already-loaded Site.
func ValidateSite(s *Site) error {
	if err := checkUniqueIDsSite(s); err != nil {
		return err
	}
	if err := checkLinksResolveSite(s); err != nil {
		return err
	}
	return checkParitySite(s)
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
	first, err := LoadSiteDir(dir)
	if err != nil {
		return err
	}
	second, err := LoadSiteDir(dir)
	if err != nil {
		return err
	}
	if len(first.Pages) != len(second.Pages) {
		return fmt.Errorf("render-determinism: page count %d != %d", len(first.Pages), len(second.Pages))
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
		if !slices.Equal(idsFrag, idsDoc) || !slices.Equal(idsDoc, idsMD) || !slices.Equal(idsDoc, idsJSON) {
			return fmt.Errorf("parity: %s section ids differ across representations: html=%v md=%v json=%v", p.Path, idsDoc, idsMD, idsJSON)
		}

		linksFrag := ExtractHTMLLinks(htmlFrag)
		linksDoc := ExtractHTMLLinks(htmlDoc)
		linksMD := ExtractMarkdownLinks(mdDoc)
		linksJSON, err := ExtractJSONLinks(jsonDoc)
		if err != nil {
			return fmt.Errorf("parity: %s: %w", p.Path, err)
		}
		if !slices.Equal(linksFrag, linksDoc) || !slices.Equal(linksDoc, linksMD) || !slices.Equal(linksDoc, linksJSON) {
			return fmt.Errorf("parity: %s links differ across representations: html=%v md=%v json=%v", p.Path, linksDoc, linksMD, linksJSON)
		}
	}
	return nil
}

// CheckUniqueIDs verifies unique page paths and unique declared section ids.
func CheckUniqueIDs(dir string) error {
	site, err := LoadSiteDir(dir)
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

// CheckLinksResolve verifies that every internal link in navigation and page
// sections resolves to a served route and (when #fragment is present) to a
// declared section id on the target page.
func CheckLinksResolve(dir string) error {
	site, err := LoadSiteDir(dir)
	if err != nil {
		return err
	}
	return checkLinksResolveSite(site)
}

func checkLinksResolveSite(s *Site) error {
	staticRoutes := map[string]bool{
		"/robots.txt": true,
		"/llms.txt":   true,
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
	baseSite, err := LoadSiteDir(baseDir)
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

func resolveWishInput(pageHint, element, wishFile, commitMsgFile string) (string, string, error) {
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
