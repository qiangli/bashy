// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SiteConfig holds hugo.toml values the generator reads.
type SiteConfig struct {
	BaseURL   string
	Title     string
	Tagline   string
	Author    string
	Language  string
	SourceDir string
	Intro     string
}

// Post is one dated entry under content/posts/.
type Post struct {
	Slug      string
	Title     string
	Date      string
	Summary   string
	Series    string
	Part      string
	Video     string
	Captions  string
	Audio     string
	Cast      string
	Draft     bool
	Body      string
	BodyHTML  string
	SeriesSib []*Post
}

// Page is one standalone page under content/.
type Page struct {
	Slug  string
	Title string
	Body  string
}

// ParseConfig reads the small hugo.toml subset (key = "value" lines).
func ParseConfig(path string) SiteConfig {
	cfg := SiteConfig{BaseURL: "/", Title: "My Journal", Tagline: "Notes in the open", Author: "The author", Language: "en"}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		for _, key := range []string{"title", "tagline", "author", "languageCode", "baseURL"} {
			if strings.HasPrefix(line, key+" = ") {
				val := strings.TrimSpace(strings.TrimPrefix(line, key+" = "))
				val = strings.Trim(val, `"'`)
				switch key {
				case "title":
					cfg.Title = val
				case "tagline":
					cfg.Tagline = val
				case "author":
					cfg.Author = val
				case "languageCode":
					cfg.Language = val
				case "baseURL":
					cfg.BaseURL = val
				}
			}
		}
	}
	return cfg
}

// LoadContent reads content/ into posts (dated entries under posts/) and
// pages (standalone markdown files), posts sorted newest first.
func LoadContent(dir string) ([]*Post, []*Page, error) {
	var posts []*Post
	var pages []*Page

	postDir := filepath.Join(dir, "posts")
	entries, err := os.ReadDir(postDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			path := filepath.Join(postDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, nil, err
			}
			post := ParsePost(string(data), strings.TrimSuffix(entry.Name(), ".md"))
			if post != nil {
				posts = append(posts, post)
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}

	sort.Slice(posts, func(i, j int) bool {
		if posts[i].Date != posts[j].Date {
			return posts[i].Date > posts[j].Date
		}
		return posts[i].Slug > posts[j].Slug
	})

	bySeries := map[string][]*Post{}
	for _, p := range posts {
		if p.Series != "" {
			bySeries[p.Series] = append(bySeries[p.Series], p)
		}
	}
	for _, siblings := range bySeries {
		for _, p := range siblings {
			p.SeriesSib = siblings
		}
	}

	top, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range top {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := entry.Name()
		if name == "_index.md" || name == "index.md" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, err
		}
		doc := ParseDoc(string(data), strings.TrimSuffix(name, ".md"))
		pages = append(pages, &Page{Slug: doc.Slug, Title: doc.Title, Body: doc.Body})
	}

	return posts, pages, nil
}

// doc is the parsed form shared by posts and pages.
type doc struct {
	Slug  string
	Title string
	Body  string
}

// ParseDoc splits front matter from a markdown document.
func ParseDoc(raw, slug string) doc {
	d := doc{Slug: slug, Title: strings.Title(strings.ReplaceAll(slug, "-", " "))}
	if strings.HasPrefix(raw, "---\n") {
		if idx := strings.Index(raw[4:], "\n---\n"); idx != -1 {
			front := raw[4 : 4+idx]
			d.Body = raw[4+idx+5:]
			for _, line := range strings.Split(front, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "title:") {
					d.Title = strings.TrimSpace(strings.Trim(strings.TrimPrefix(line, "title:"), `"'`))
				}
			}
			return d
		}
	}
	d.Body = raw
	return d
}

// ParsePost parses a post with its front matter; nil for drafts.
func ParsePost(raw, slug string) *Post {
	base := ParseDoc(raw, slug)
	p := &Post{Slug: slug, Title: base.Title, Body: base.Body}
	if strings.HasPrefix(raw, "---\n") {
		if idx := strings.Index(raw[4:], "\n---\n"); idx != -1 {
			for _, line := range strings.Split(raw[4:4+idx], "\n") {
				line = strings.TrimSpace(line)
				val := ""
				if i := strings.Index(line, ":"); i > 0 {
					val = strings.TrimSpace(line[i+1:])
					val = strings.Trim(val, `"'`)
				} else {
					continue
				}
				key := strings.TrimSpace(line[:strings.Index(line, ":")])
				switch key {
				case "title":
					p.Title = val
				case "date":
					p.Date = val
				case "summary":
					p.Summary = val
				case "series":
					p.Series = val
				case "part":
					p.Part = val
				case "video":
					p.Video = val
				case "captions":
					p.Captions = val
				case "audio":
					p.Audio = val
				case "cast":
					p.Cast = val
				case "draft":
					p.Draft = val == "true"
				}
			}
		}
	}
	if p.Draft {
		return nil
	}
	if p.Date == "" {
		if len(slug) >= 10 && slug[4] == '-' && slug[7] == '-' {
			p.Date = slug[:10]
		}
	}
	p.BodyHTML = Markdown(p.Body)
	return p
}

// Markdown renders the blog's markdown subset, including the rich fences:
// ```diagram, ```math, fenced code with syntax coloring, and $inline math$.
func Markdown(md string) string {
	var out strings.Builder
	lines := strings.Split(md, "\n")
	i := 0
	listOpen := false
	para := []string{}

	flushPara := func() {
		if len(para) == 0 {
			return
		}
		fmt.Fprintf(&out, "<p>%s</p>\n", inline(strings.Join(para, " ")))
		para = para[:0]
	}
	closeList := func() {
		if listOpen {
			out.WriteString("</ul>\n")
			listOpen = false
		}
	}

	for i < len(lines) {
		line := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(line)

		if trimmed == "$$" {
			flushPara()
			closeList()
			var body []string
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != "$$" {
				body = append(body, lines[i])
				i++
			}
			i++
			fmt.Fprintf(&out, "<div class=\"math math-block\">%s</div>\n", MathHTML(strings.Join(body, " ")))
			continue
		}
		if strings.HasPrefix(trimmed, "$$") && strings.HasSuffix(trimmed, "$$") && len(trimmed) > 4 {
			flushPara()
			closeList()
			fmt.Fprintf(&out, "<div class=\"math math-block\">%s</div>\n", MathHTML(trimmed[2:len(trimmed)-2]))
			i++
			continue
		}

		if fence, info, ok := splitFence(line); ok && fence == "```" {
			flushPara()
			closeList()
			var body []string
			i++
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				body = append(body, lines[i])
				i++
			}
			i++
			out.WriteString(renderFence(info, strings.Join(body, "\n")))
			continue
		}

		if trimmed == "" {
			flushPara()
			closeList()
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			flushPara()
			if !listOpen {
				out.WriteString("<ul>\n")
				listOpen = true
			}
			fmt.Fprintf(&out, "<li>%s</li>\n", inline(strings.TrimSpace(trimmed[2:])))
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "> ") {
			flushPara()
			closeList()
			fmt.Fprintf(&out, "<blockquote>%s</blockquote>\n", inline(strings.TrimSpace(trimmed[2:])))
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "### ") {
			flushPara()
			closeList()
			fmt.Fprintf(&out, "<h3>%s</h3>\n", inline(trimmed[4:]))
			i++
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			flushPara()
			closeList()
			fmt.Fprintf(&out, "<h2>%s</h2>\n", inline(trimmed[3:]))
			i++
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			flushPara()
			closeList()
			fmt.Fprintf(&out, "<h1>%s</h1>\n", inline(trimmed[2:]))
			i++
			continue
		}

		closeList()
		para = append(para, trimmed)
		i++
	}
	flushPara()
	closeList()
	return out.String()
}

// splitFence reports a fence opener and its info word.
func splitFence(line string) (fence, info string, ok bool) {
	if strings.HasPrefix(line, "```") {
		return "```", strings.TrimSpace(strings.TrimPrefix(line, "```")), true
	}
	return "", "", false
}

// renderFence dispatches one fenced block to its renderer.
func renderFence(info, body string) string {
	switch strings.ToLower(info) {
	case "diagram":
		svg, err := DiagramSVG(body)
		if err != nil {
			return fmt.Sprintf("<pre class=\"diagram-error\">%s</pre>\n", html.EscapeString(err.Error()))
		}
		return fmt.Sprintf("<figure class=\"diagram\">%s</figure>\n", svg)
	case "math", "mathblock":
		return fmt.Sprintf("<div class=\"math math-block\">%s</div>\n", MathHTML(body))
	default:
		lang := strings.ToLower(strings.SplitN(info, " ", 2)[0])
		return fmt.Sprintf("<pre class=\"code lang-%s\"><code>%s</code></pre>\n", html.EscapeString(lang), Highlight(lang, body))
	}
}

// inline renders inline markdown: emphasis, code, links, images, $math$.
func inline(text string) string {
	text = html.EscapeString(text)

	var out strings.Builder
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '$':
			if end := strings.IndexByte(text[i+1:], '$'); end >= 0 && end > 0 {
				expr := text[i+1 : i+1+end]
				if !strings.ContainsAny(expr, "\n") && !strings.HasPrefix(expr, " ") && !strings.HasSuffix(expr, " ") {
					fmt.Fprintf(&out, "<span class=\"math\">%s</span>", MathHTML(expr))
					i += end + 1
					continue
				}
			}
			out.WriteByte('$')
		case '`':
			if end := strings.IndexByte(text[i+1:], '`'); end >= 0 {
				fmt.Fprintf(&out, "<code>%s</code>", text[i+1:i+1+end])
				i += end + 1
				continue
			}
			out.WriteByte('`')
		case '!':
			if strings.HasPrefix(text[i+1:], "[") {
				if m := linkMatch(text[i+1:]); m != nil {
					fmt.Fprintf(&out, "<img src=\"%s\" alt=\"%s\" loading=\"lazy\">", m[2], m[1])
					i += len(m[0])
					continue
				}
			}
			out.WriteByte('!')
		case '[':
			if m := linkMatch(text[i:]); m != nil {
				fmt.Fprintf(&out, "<a href=\"%s\">%s</a>", m[2], m[1])
				i += len(m[0]) - 1
				continue
			}
			out.WriteByte('[')
		case '*', '_':
			c := text[i]
			if i+1 < len(text) && text[i+1] == c {
				if end := strings.IndexByte(text[i+2:], c); end > 0 && text[i+2+end+1] == c {
					fmt.Fprintf(&out, "<strong>%s</strong>", text[i+2:i+2+end])
					i += end + 3
					continue
				}
			} else if canOpenEmphasis(text, i, c) {
				if end := findEmphasisClose(text, i+1, c); end > 0 {
					fmt.Fprintf(&out, "<em>%s</em>", text[i+1:i+1+end])
					i += end + 1
					continue
				}
			}
			out.WriteByte(c)
		default:
			out.WriteByte(text[i])
		}
	}

	return out.String()
}

// canOpenEmphasis keeps underscores inside words (snake_case) literal.
func canOpenEmphasis(text string, i int, c byte) bool {
	if i+1 >= len(text) || text[i+1] == ' ' || text[i+1] == c {
		return false
	}
	if c == '_' {
		return i == 0 || text[i-1] == ' ' || text[i-1] == '(' || text[i-1] == '>'
	}
	return true
}

// findEmphasisClose finds a closing emphasis marker not preceded by a space.
func findEmphasisClose(text string, from int, c byte) int {
	for j := from; j < len(text); j++ {
		if text[j] == c && text[j-1] != ' ' && (j+1 >= len(text) || text[j+1] == ' ' || text[j+1] == '.' || text[j+1] == ',' || text[j+1] == ')') {
			return j - from
		}
	}
	return -1
}

// linkMatch extracts [text](url) at the start of s.
func linkMatch(s string) []string {
	if !strings.HasPrefix(s, "[") {
		return nil
	}
	close := strings.IndexByte(s, ']')
	if close < 0 || close+1 >= len(s) || s[close+1] != '(' {
		return nil
	}
	end := strings.IndexByte(s[close+1:], ')')
	if end < 0 {
		return nil
	}
	return []string{s[:close+2+end], s[1:close], s[close+2 : close+1+end]}
}

// seriesBadge renders the series line and sibling links for a post.
func seriesBadge(p *Post) string {
	if p.Series == "" {
		return ""
	}
	label := html.EscapeString(p.Series)
	if p.Part != "" {
		label += ", part " + html.EscapeString(p.Part)
	}
	var links []string
	for _, sib := range p.SeriesSib {
		if sib.Slug == p.Slug {
			continue
		}
		links = append(links, fmt.Sprintf("<a href=\"../%s/\">%s</a>", sib.Slug, html.EscapeString(sib.Title)))
	}
	nav := ""
	if len(links) > 0 {
		nav = fmt.Sprintf("<div class=\"series-nav\">Also in this series: %s</div>", strings.Join(links, " &middot; "))
	}
	return fmt.Sprintf("<div class=\"series-badge\">Series: %s</div>\n%s", label, nav)
}

// mediaBlock renders the video player with its caption track, the audio
// player and the terminal-scene transcript from a post's front matter.
func mediaBlock(p *Post) string {
	var out strings.Builder
	if p.Video != "" {
		track := ""
		if p.Captions != "" {
			track = fmt.Sprintf("<track kind=\"captions\" src=\"../../media/%s\" srclang=\"en\" label=\"English\" default>", html.EscapeString(filepath.Base(p.Captions)))
		}
		fmt.Fprintf(&out, "<video class=\"post-video\" controls preload=\"metadata\" src=\"../../media/%s\">%s</video>\n", html.EscapeString(filepath.Base(p.Video)), track)
	}
	if p.Audio != "" {
		fmt.Fprintf(&out, "<audio class=\"post-audio\" controls preload=\"metadata\" src=\"../../media/%s\"></audio>\n", html.EscapeString(filepath.Base(p.Audio)))
	}
	if p.Cast != "" {
		if html_, err := CastTranscript(p.Cast); err != nil {
			fmt.Fprintf(&out, "<pre class=\"cast-error\">%s</pre>\n", html.EscapeString(err.Error()))
		} else {
			out.WriteString(html_)
		}
	}
	return out.String()
}

// layout wraps body HTML in the site document.
func layout(cfg SiteConfig, title, body string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="%s">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="generator" content="bashy blog">
<title>%s</title>
<link rel="stylesheet" href="/css/style.css">
</head>
<body>
<div class="site-wrapper">
<header class="site-header">
  <a class="brand" href="/">%s</a>
  <p class="tagline">%s</p>
</header>
<nav class="site-nav" aria-label="Site">
  <a href="/">Posts</a>
  <a href="/about/">About</a>
  <a href="/index.xml">RSS</a>
  <a class="nav-admin" href="/admin/">Admin CMS</a>
</nav>
<main class="site-main">
%s
</main>
<footer class="site-footer">
  <p>&copy; 2026 %s &middot; served from the author's own machine via <a href="https://github.com/qiangli/bashy">bashy</a></p>
</footer>
</div>
</body>
</html>`, html.EscapeString(cfg.Language), html.EscapeString(title), html.EscapeString(cfg.Title), html.EscapeString(cfg.Tagline), body, html.EscapeString(cfg.Author))
}

// RenderPost writes the full HTML document for one post.
func RenderPost(cfg SiteConfig, p *Post, all []*Post) string {
	body := fmt.Sprintf(`<article class="post">
<header class="post-header">
  <h1>%s</h1>
  <time class="post-date" datetime="%s">%s</time>
</header>
%s
%s
<div class="post-content">
%s
</div>
</article>`, html.EscapeString(p.Title), html.EscapeString(p.Date), html.EscapeString(p.Date), seriesBadge(p), mediaBlock(p), p.BodyHTML)
	return layout(cfg, p.Title+" · "+cfg.Title, body)
}

// RenderPage writes the full HTML document for a standalone page.
func RenderPage(cfg SiteConfig, p *Page) string {
	body := fmt.Sprintf(`<article class="page">
<header class="post-header"><h1>%s</h1></header>
<div class="post-content">
%s
</div>
</article>`, html.EscapeString(p.Title), Markdown(p.Body))
	return layout(cfg, p.Title+" · "+cfg.Title, body)
}

// RenderHome writes the home page: intro plus the post list, newest first.
func RenderHome(cfg SiteConfig, posts []*Post) string {
	var list strings.Builder
	for _, p := range posts {
		series := ""
		if p.Series != "" {
			series = fmt.Sprintf("<span class=\"series-chip\">%s</span>", html.EscapeString(p.Series))
		}
		summary := ""
		if p.Summary != "" {
			summary = fmt.Sprintf("<p class=\"post-summary\">%s</p>", html.EscapeString(p.Summary))
		}
		fmt.Fprintf(&list, `<article class="post-card">
<h2><a href="p/%s/">%s</a></h2>
<time datetime="%s">%s</time> %s
%s
</article>
`, p.Slug, html.EscapeString(p.Title), html.EscapeString(p.Date), html.EscapeString(p.Date), series, summary)
	}
	intro := ""
	if cfg.Intro != "" {
		intro = fmt.Sprintf("<section class=\"home-intro\">%s</section>\n", Markdown(cfg.Intro))
	}
	body := fmt.Sprintf("%s<section class=\"post-list\">\n%s</section>\n<p class=\"feed-hint\">Subscribe with the <a href=\"index.xml\">RSS feed</a>.</p>\n", intro, list.String())
	return layout(cfg, cfg.Title+" — "+cfg.Tagline, body)
}

func copyDir(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
