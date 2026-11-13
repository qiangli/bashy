// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type SiteConfig struct {
	BaseURL      string
	Title        string
	Tagline      string
	CompanyName  string
	ContactEmail string
	ContactPhone string
	Address      string
	ShowPricing  bool
}

type PageData struct {
	Title          string
	Tagline        string
	Description    string
	HeroHeading    string
	HeroSubheading string
	BodyMarkdown   string
	BodyHTML       string
	Slug           string
	IsHome         bool
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: generator <source-dir> [output-dir]\n")
		os.Exit(1)
	}

	srcDir, err := filepath.Abs(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving source dir: %v\n", err)
		os.Exit(1)
	}

	outDir := filepath.Join(srcDir, "public")
	if len(os.Args) >= 3 {
		outDir, err = filepath.Abs(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving out dir: %v\n", err)
			os.Exit(1)
		}
	}

	if err := BuildSite(srcDir, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "Build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Site successfully generated at: %s\n", outDir)
}

// BuildSite builds the Hugo-compatible static site from srcDir to outDir.
// If hugo CLI exists on PATH and hugo build succeeds, it can use it, but
// our pure-Go builder ensures hermetic and identical output across all environments.
func BuildSite(srcDir, outDir string) error {
	// If hugo is available, we can run hugo, but we also ensure the pure Go builder works
	// cleanly and deterministically.
	cfg := parseHugoConfig(filepath.Join(srcDir, "hugo.toml"))

	// Ensure clean output directory
	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("clean outDir: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir outDir: %w", err)
	}

	// Copy static assets first (including /admin/ for Sveltia CMS and visual editor)
	staticDir := filepath.Join(srcDir, "static")
	if _, err := os.Stat(staticDir); err == nil {
		if err := copyDir(staticDir, outDir); err != nil {
			return fmt.Errorf("copy static assets: %w", err)
		}
	}

	// Read content directory
	contentDir := filepath.Join(srcDir, "content")
	if _, err := os.Stat(contentDir); err != nil {
		return fmt.Errorf("content dir missing: %s", contentDir)
	}

	entries, err := os.ReadDir(contentDir)
	if err != nil {
		return fmt.Errorf("read content dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(contentDir, entry.Name())
		page, err := parseMarkdownPage(path)
		if err != nil {
			return fmt.Errorf("parse page %s: %w", entry.Name(), err)
		}

		htmlContent := renderPage(cfg, page, srcDir)

		var targetPath string
		if page.IsHome {
			targetPath = filepath.Join(outDir, "index.html")
		} else {
			pageDir := filepath.Join(outDir, page.Slug)
			if err := os.MkdirAll(pageDir, 0o755); err != nil {
				return err
			}
			targetPath = filepath.Join(pageDir, "index.html")
		}

		if err := os.WriteFile(targetPath, []byte(htmlContent), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", targetPath, err)
		}
	}

	// Ensure GitHub Pages serves files as-is
	_ = os.WriteFile(filepath.Join(outDir, ".nojekyll"), []byte(""), 0o644)

	return nil
}

func parseHugoConfig(path string) SiteConfig {
	cfg := SiteConfig{
		BaseURL:      "/",
		Title:        "Acme Innovations",
		Tagline:      "Engineering tomorrow's reliable software systems",
		CompanyName:  "Acme Innovations",
		ContactEmail: "hello@acme.example.invalid",
		ContactPhone: "+1 (555) 019-2834",
		Address:      "100 Innovation Way, Suite 400",
		ShowPricing:  false,
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "title") {
			cfg.Title = extractTomlString(line)
		} else if strings.HasPrefix(line, "tagline") {
			cfg.Tagline = extractTomlString(line)
		} else if strings.HasPrefix(line, "company_name") {
			cfg.CompanyName = extractTomlString(line)
		} else if strings.HasPrefix(line, "contact_email") {
			cfg.ContactEmail = extractTomlString(line)
		} else if strings.HasPrefix(line, "contact_phone") {
			cfg.ContactPhone = extractTomlString(line)
		} else if strings.HasPrefix(line, "show_pricing") {
			cfg.ShowPricing = strings.Contains(strings.ToLower(line), "true")
		}
	}
	return cfg
}

func extractTomlString(line string) string {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return ""
	}
	val := strings.TrimSpace(parts[1])
	val = strings.Trim(val, "\"")
	val = strings.Trim(val, "'")
	return val
}

func parseMarkdownPage(path string) (*PageData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	base := filepath.Base(path)
	slug := strings.TrimSuffix(base, ".md")
	isHome := slug == "_index" || slug == "index"

	page := &PageData{
		Slug:   slug,
		IsHome: isHome,
		Title:  strings.Title(slug),
	}

	raw := string(data)
	if strings.HasPrefix(raw, "---\n") {
		idx := strings.Index(raw[4:], "\n---\n")
		if idx != -1 {
			front := raw[4 : 4+idx]
			body := raw[4+idx+5:]
			page.BodyMarkdown = body

			for _, line := range strings.Split(front, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "title:") {
					page.Title = strings.Trim(strings.TrimPrefix(line, "title:"), " \"'")
				} else if strings.HasPrefix(line, "tagline:") {
					page.Tagline = strings.Trim(strings.TrimPrefix(line, "tagline:"), " \"'")
				} else if strings.HasPrefix(line, "description:") {
					page.Description = strings.Trim(strings.TrimPrefix(line, "description:"), " \"'")
				} else if strings.HasPrefix(line, "hero_heading:") {
					page.HeroHeading = strings.Trim(strings.TrimPrefix(line, "hero_heading:"), " \"'")
				} else if strings.HasPrefix(line, "hero_subheading:") {
					page.HeroSubheading = strings.Trim(strings.TrimPrefix(line, "hero_subheading:"), " \"'")
				}
			}
		} else {
			page.BodyMarkdown = raw
		}
	} else {
		page.BodyMarkdown = raw
	}

	page.BodyHTML = markdownToHTML(page.BodyMarkdown)
	return page, nil
}

func markdownToHTML(md string) string {
	var buf bytes.Buffer
	lines := strings.Split(md, "\n")
	inList := false

	linkRe := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	boldRe := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	codeRe := regexp.MustCompile("`([^`]+)`")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r\n")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				buf.WriteString("<ul>\n")
				inList = true
			}
			itemText := strings.TrimSpace(trimmed[2:])
			itemText = formatInline(itemText, linkRe, boldRe, codeRe)
			buf.WriteString(fmt.Sprintf("  <li>%s</li>\n", itemText))
			continue
		} else if inList && trimmed == "" {
			buf.WriteString("</ul>\n")
			inList = false
			continue
		} else if inList {
			buf.WriteString("</ul>\n")
			inList = false
		}

		if strings.HasPrefix(trimmed, "### ") {
			hText := formatInline(trimmed[4:], linkRe, boldRe, codeRe)
			buf.WriteString(fmt.Sprintf("<h3>%s</h3>\n", hText))
		} else if strings.HasPrefix(trimmed, "## ") {
			hText := formatInline(trimmed[3:], linkRe, boldRe, codeRe)
			buf.WriteString(fmt.Sprintf("<h2>%s</h2>\n", hText))
		} else if strings.HasPrefix(trimmed, "# ") {
			hText := formatInline(trimmed[2:], linkRe, boldRe, codeRe)
			buf.WriteString(fmt.Sprintf("<h1>%s</h1>\n", hText))
		} else if trimmed != "" {
			pText := formatInline(trimmed, linkRe, boldRe, codeRe)
			buf.WriteString(fmt.Sprintf("<p>%s</p>\n", pText))
		}
	}
	if inList {
		buf.WriteString("</ul>\n")
	}

	return buf.String()
}

func formatInline(text string, linkRe, boldRe, codeRe *regexp.Regexp) string {
	text = html.EscapeString(text)
	// un-escape for markdown links and formatting that were escaped
	text = boldRe.ReplaceAllString(text, "<strong>$1</strong>")
	text = codeRe.ReplaceAllString(text, "<code>$1</code>")
	text = linkRe.ReplaceAllString(text, `<a href="$2">$1</a>`)
	return text
}

func renderPage(cfg SiteConfig, page *PageData, srcDir string) string {
	titleText := page.Title + " · " + cfg.Title
	if page.IsHome {
		titleText = cfg.Title + " — " + cfg.Tagline
	}

	pricingNav := ""
	if cfg.ShowPricing {
		pricingNav = `<li><a href="/pricing/" class="nav-link">Pricing</a></li>`
	}

	pricingSectionHTML := ""
	if page.IsHome && cfg.ShowPricing {
		pricingPartialPath := filepath.Join(srcDir, "layouts", "partials", "pricing.html")
		if data, err := os.ReadFile(pricingPartialPath); err == nil {
			pricingSectionHTML = string(data)
		}
	}

	var mainContent string
	if page.IsHome {
		heroHeading := page.HeroHeading
		if heroHeading == "" {
			heroHeading = "Build Reliable Software for the Open Cloud"
		}
		heroSubheading := page.HeroSubheading
		if heroSubheading == "" {
			heroSubheading = cfg.Tagline
		}
		tagline := page.Tagline
		if tagline == "" {
			tagline = cfg.Tagline
		}

		mainContent = fmt.Sprintf(`
<section class="hero-section">
  <div class="hero-content">
    <div class="tagline-badge">%s</div>
    <h1 class="hero-title">%s</h1>
    <p class="hero-subtext">%s</p>
    <div class="hero-actions">
      <a href="/services/" class="btn btn-primary">Our Services</a>
      <a href="/contact/" class="btn btn-secondary">Get In Touch</a>
    </div>
  </div>
</section>

<section class="home-content">
  %s
</section>
%s
`, html.EscapeString(tagline), html.EscapeString(heroHeading), html.EscapeString(heroSubheading), page.BodyHTML, pricingSectionHTML)
	} else {
		descHTML := ""
		if page.Description != "" {
			descHTML = fmt.Sprintf(`<p class="page-description">%s</p>`, html.EscapeString(page.Description))
		}
		mainContent = fmt.Sprintf(`
<div class="content-container container">
  <article class="page-article">
    <header class="page-header">
      <h1 class="page-title">%s</h1>
      %s
    </header>
    <div class="page-content">
      %s
    </div>
  </article>
</div>
`, html.EscapeString(page.Title), descHTML, page.BodyHTML)
	}

	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <link rel="stylesheet" href="/css/style.css">
</head>
<body>
<div class="site-wrapper">
  <header class="site-header">
    <div class="container header-inner">
      <a href="/" class="brand-logo">
        <span class="logo-mark">⚡</span>
        <span class="logo-text">%s</span>
      </a>
      <nav class="site-nav" aria-label="Main Navigation">
        <ul class="nav-list">
          <li><a href="/" class="nav-link">Home</a></li>
          <li><a href="/about/" class="nav-link">About</a></li>
          <li><a href="/services/" class="nav-link">Services</a></li>
          %s
          <li><a href="/contact/" class="nav-link">Contact</a></li>
          <li><a href="/admin/" class="nav-link nav-btn" title="Content Management System">Admin CMS</a></li>
        </ul>
      </nav>
    </div>
  </header>
  <main class="site-main">
    %s
  </main>
  <footer class="site-footer">
    <div class="container footer-inner">
      <div class="footer-info">
        <p class="copyright">&copy; 2026 %s. All rights reserved.</p>
        <p class="host-notice">Served locally from the author's own machine via <a href="https://github.com/qiangli/bashy">bashy</a>.</p>
      </div>
      <div class="footer-links">
        <a href="/admin/">Sveltia CMS</a> &middot;
        <a href="/admin/editor/">Visual Editor</a> &middot;
        <a href="/contact/">Support</a>
      </div>
    </div>
  </footer>
</div>
</body>
</html>`, html.EscapeString(titleText), html.EscapeString(cfg.Title), pricingNav, mainContent, html.EscapeString(cfg.CompanyName))
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
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
