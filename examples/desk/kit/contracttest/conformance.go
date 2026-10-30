package contracttest

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	kit "desk-kit"
)

var (
	styleBlockRe = regexp.MustCompile(`(?is)<style[^>]*>(.*?)</style>`)
	scriptRe     = regexp.MustCompile(`(?is)<script[^>]*>(.*?)</script>`)
	styleAttrRe  = regexp.MustCompile(`(?is)\sstyle\s*=\s*("[^"]*"|'[^']*')`)
	// SVG/HTML presentation attributes that carry a colour or a font.
	presAttrRe = regexp.MustCompile(`(?is)\s(fill|stroke|stop-color|color|bgcolor)\s*=\s*"(#[0-9a-f]{3,8})"`)
	fontAttrRe = regexp.MustCompile(`(?is)\sfont-family\s*=\s*"[^"]*"`)
	hexRe      = regexp.MustCompile(`(?i)#[0-9a-f]{3,8}\b`)
	funcColRe  = regexp.MustCompile(`(?i)\b(rgba?|hsla?)\(`)
	// Script heuristics: a colour-ish property assigned/declared with a
	// literal, and font family assignments (style.fontFamily, ctx.font).
	scriptColRe  = regexp.MustCompile(`(?i)\b(color|background|fill|stroke|border|shadow)\w*\s*[:=][^;\n{}]*?(#[0-9a-f]{3,8}\b|\b(?:rgba?|hsla?)\()`)
	scriptFontRe = regexp.MustCompile(`(?i)(font-family\s*:|\bfontFamily\s*=|\.font\s*=\s*['"])`)
)

// StyleViolations reports what in a served page bypasses the kit look:
// a missing kit stylesheet or shell, and — outside the kit's own CSS/JS —
// any hex/rgb/hsl colour or font-family in <style> blocks, style="…"
// attributes, presentation attributes and inline scripts. allow lists
// user-content colours (whiteboard pens); the kit data palette (chart
// series) is always allowed. Pure function, so a planted violation can be
// proven red without a server.
func StyleViolations(page string, allow []string) []string {
	var out []string
	if !strings.Contains(page, kit.CSS()) {
		out = append(out, "page does not load kit.css (desk-kit tokens missing)")
	}
	if !strings.Contains(page, `class="`+kit.MarkerAppBar+`"`) {
		out = append(out, "page lacks the kit app shell (.desk-appbar top bar)")
	}
	tool := strings.ReplaceAll(page, kit.CSS(), "")
	tool = strings.ReplaceAll(tool, kit.JS(), "")
	return append(out, FragmentViolations(tool, allow)...)
}

// FragmentViolations is the tool-CSS half of StyleViolations for markup
// that is not a whole page (a chart SVG, an embed): colour literals and
// font families outside the kit tokens and the allowlist.
func FragmentViolations(tool string, allow []string) []string {
	var out []string
	ok := map[string]bool{}
	for _, c := range append(append([]string{}, kit.DataPalette...), allow...) {
		ok[strings.ToLower(c)] = true
	}
	seen := map[string]bool{}
	add := func(where, what string) {
		k := where + ": " + what
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, m := range styleBlockRe.FindAllStringSubmatch(tool, -1) {
		for _, d := range declarations(m[1]) {
			if v := declViolation(d, ok); v != "" {
				add("<style>", v)
			}
		}
	}
	for _, m := range styleAttrRe.FindAllStringSubmatch(tool, -1) {
		for _, d := range declarations(m[1][1 : len(m[1])-1]) {
			if v := declViolation(d, ok); v != "" {
				add("style attribute", v)
			}
		}
	}
	noScript := scriptRe.ReplaceAllString(tool, "")
	for _, m := range presAttrRe.FindAllStringSubmatch(noScript, -1) {
		if !ok[strings.ToLower(m[2])] {
			add("presentation attribute", fmt.Sprintf("%s=%q uses a colour literal", m[1], m[2]))
		}
	}
	for _, m := range fontAttrRe.FindAllString(noScript, -1) {
		add("presentation attribute", strings.TrimSpace(m)+" sets its own font")
	}
	for _, m := range scriptRe.FindAllStringSubmatch(tool, -1) {
		src := m[1]
		for _, c := range scriptColRe.FindAllStringSubmatch(src, -1) {
			lit := c[2]
			if strings.HasPrefix(lit, "#") && ok[strings.ToLower(lit)] {
				continue
			}
			add("<script>", fmt.Sprintf("colour literal %q in %q", lit, trunc(strings.TrimSpace(c[0]), 60)))
		}
		for _, f := range scriptFontRe.FindAllString(src, -1) {
			add("<script>", fmt.Sprintf("sets a font family (%s); use var(--desk-font)", strings.TrimSpace(f)))
		}
	}
	sort.Strings(out)
	return out
}

// declarations splits CSS into "prop:value" declarations, dropping
// selectors, at-rule preludes and comments. Nested blocks (@media) work.
func declarations(css string) []string {
	for {
		i := strings.Index(css, "/*")
		if i < 0 {
			break
		}
		j := strings.Index(css[i+2:], "*/")
		if j < 0 {
			css = css[:i]
			break
		}
		css = css[:i] + css[i+2+j+2:]
	}
	var out []string
	var buf strings.Builder
	for _, r := range css {
		switch r {
		case '{':
			buf.Reset() // selector or at-rule prelude
		case '}', ';':
			if d := strings.TrimSpace(buf.String()); strings.Contains(d, ":") {
				out = append(out, d)
			}
			buf.Reset()
		default:
			buf.WriteRune(r)
		}
	}
	if d := strings.TrimSpace(buf.String()); strings.Contains(d, ":") {
		out = append(out, d)
	}
	return out
}

func declViolation(d string, ok map[string]bool) string {
	i := strings.Index(d, ":")
	prop := strings.ToLower(strings.TrimSpace(d[:i]))
	val := strings.TrimSpace(d[i+1:])
	lval := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(val, "!important")), " "))
	for _, h := range hexRe.FindAllString(val, -1) {
		if !ok[strings.ToLower(h)] {
			return fmt.Sprintf("%q uses colour literal %s; use a --desk-* token", trunc(d, 60), h)
		}
	}
	if funcColRe.MatchString(val) {
		return fmt.Sprintf("%q uses a colour function; use a --desk-* token", trunc(d, 60))
	}
	switch prop {
	case "font-family", "font":
		switch lval {
		case "inherit", "initial", "unset", "revert":
			return ""
		}
		if !strings.Contains(lval, "var(--desk-font") {
			return fmt.Sprintf("%q sets its own font; use var(--desk-font)", trunc(d, 60))
		}
	}
	return ""
}
