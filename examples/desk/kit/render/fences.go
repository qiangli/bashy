package render

import (
	"bytes"
	"fmt"
	"html"
	"strconv"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"
)

// ChartSVG renders a ```chart fence body to an SVG fragment. It is a hook
// so the full desk chart (C1) renderer can be wired in once examples/desk
// exposes it as an importable func: tools that vendor it set
//
//	render.ChartSVG = chart.RenderFence
//
// and every chart fence uses the C1 renderer. Until then the built-in
// compact renderer below (same CSV/JSON input shapes, C1-style geometry)
// serves fences server-side with no extra dependency.
var ChartSVG func(widget, format, title string, data []byte) (string, error)

// renderFence implements the fenced-code half of deskOverrides.
func (d *deskOverrides) renderFence(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.FencedCodeBlock)
	lang := string(n.Language(source))
	info := ""
	if n.Info != nil {
		info = string(n.Info.Text(source))
	}
	var body bytes.Buffer
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		body.Write(seg.Value(source))
	}
	code := body.Bytes()
	if len(code) > 64<<10 {
		code = code[:64<<10]
	}

	switch strings.ToLower(lang) {
	case "chart":
		fmt.Fprint(w, d.chartFence(info, code))
	case "csv", "tsv":
		fmt.Fprint(w, csvFence(lang, code))
	default:
		fmt.Fprint(w, highlight(lang, code))
	}
	return ast.WalkContinue, nil
}

// chartParams are the space-separated key=value pairs after ```chart
// (widget, format, title; values may be double-quoted).
func chartParams(info string) map[string]string {
	out := map[string]string{}
	fields := splitParams(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(info), "chart")))
	for _, f := range fields {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		if unq, err := strconv.Unquote(strings.TrimSpace(v)); err == nil {
			v = unq
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out
}

// splitParams splits on spaces honouring double quotes.
func splitParams(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			cur.WriteRune(r)
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// chartFence renders a ```chart fence to a server-side SVG via the ChartSVG
// hook (or the built-in renderer). Bad input degrades to a marked code
// block, never an error and never raw data passthrough.
func (d *deskOverrides) chartFence(info string, data []byte) string {
	params := chartParams(info)
	widget, format, title := params["widget"], params["format"], params["title"]
	svg := ""
	if ChartSVG != nil {
		s, err := ChartSVG(widget, format, title, data)
		if err == nil {
			svg = s
		} else {
			return chartError(title, err.Error(), data)
		}
	} else {
		s, err := renderChartSVG(widget, format, title, data)
		if err != nil {
			return chartError(title, err.Error(), data)
		}
		svg = s
	}
	var b strings.Builder
	b.WriteString(`<div class="desk-embed desk-embed-chart">`)
	if title != "" {
		b.WriteString(`<p class="desk-embed-caption">` + html.EscapeString(title) + `</p>`)
	}
	b.WriteString(svg)
	b.WriteString(`</div>`)
	return b.String()
}

// chartError is the graceful chart fallback: the message plus the source
// data as plain escaped code.
func chartError(title, msg string, data []byte) string {
	var b strings.Builder
	b.WriteString(`<div class="desk-embed desk-embed-chart desk-embed-error">`)
	if title != "" {
		b.WriteString(`<p class="desk-embed-caption">` + html.EscapeString(title) + `</p>`)
	}
	b.WriteString(`<p class="desk-embed-note">` + html.EscapeString(msg) + `</p>`)
	b.WriteString(`<pre class="desk-code"><code>` + html.EscapeString(string(data)) + "</code></pre></div>")
	return b.String()
}

// csvFence renders a fenced ```csv / ```tsv block as an inline table
// (same shape as a csv embed, minus the download link: fenced data has
// no file to download).
func csvFence(lang string, data []byte) string {
	delim := ','
	if strings.ToLower(lang) == "tsv" {
		delim = '\t'
	}
	inner := csvTable(data, delim, "", "")
	// csvTable with no href emits a download link to ""; drop it: there
	// is no file behind fenced data.
	if idx := strings.LastIndex(inner, `<a class="desk-download"`); idx >= 0 {
		if end := strings.Index(inner[idx:], `</a>`); end >= 0 {
			inner = inner[:idx] + inner[idx+end+len(`</a>`):]
		}
	}
	return inner
}

// highlight renders any other fenced block with server-side highlighting
// (chroma, class-based spans prefixed desk-hl-). Unknown languages and
// oversized blocks fall back to plain escaped code. Output carries
// desk-code plus a language class; colour itself belongs to kit.css.
func highlight(lang string, code []byte) string {
	langClass := ""
	if lang != "" {
		langClass = ` class="language-` + html.EscapeString(lang) + `"`
	}
	plain := `<pre class="desk-code"><code` + langClass + `>` +
		html.EscapeString(string(code)) + "</code></pre>"
	if lang == "" || bytes.Count(code, []byte{'\n'}) > 2000 {
		return plain
	}
	lexer := lexers.Get(strings.ToLower(lang))
	if lexer == nil {
		return plain
	}
	it, err := lexer.Tokenise(nil, string(code))
	if err != nil {
		return plain
	}
	// Class-based spans only: no inline styles, so the view CSP holds.
	htmlFmt := chromahtml.New(
		chromahtml.WithClasses(true),
		chromahtml.ClassPrefix("desk-hl-"),
		chromahtml.PreventSurroundingPre(true),
	)
	var b bytes.Buffer
	if err := htmlFmt.Format(&b, styles.Fallback, it); err != nil {
		return plain
	}
	return `<pre class="desk-code"><code` + langClass + `>` + b.String() + "</code></pre>"
}
