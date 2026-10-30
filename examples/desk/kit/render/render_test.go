package render

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var errTestChart = errors.New("test chart failure")

// fixture builds an asset dir with one file per embed type.
func fixture(t *testing.T) (dir string, opt Options) {
	t.Helper()
	dir = t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("data.csv", "name,age\nada,36\ngrace,85\n")
	write("data.tsv", "name\tage\nada\t36\n")
	rows := "h1,h2\n"
	for i := 0; i < 30; i++ {
		rows += "a,b\n"
	}
	write("big.csv", rows)
	write("pic.png", "\x89PNG\r\n\x1a\n")
	write("pic.jpg", "\xff\xd8\xff")
	write("pic.svg", `<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	write("clip.mp4", "fake-mp4")
	write("clip.webm", "fake-webm")
	write("doc.pdf", "%PDF-1.4 fake")
	write("notes.txt", "plain text")
	return dir, Options{AssetDir: dir, AssetURL: "/assets/x/"}
}

func render(t *testing.T, src string, opt Options) string {
	t.Helper()
	out, err := Markdown([]byte(src), opt)
	if err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	return string(out)
}

func mustContain(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output lacks %q\noutput:\n%s", w, out)
		}
	}
}

func mustLack(t *testing.T, out string, banned ...string) {
	t.Helper()
	for _, b := range banned {
		if strings.Contains(out, b) {
			t.Errorf("output contains banned %q\noutput:\n%s", b, out)
		}
	}
}

func TestGFMFeatures(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "# Title\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n- [x] done\n- [ ] todo\n\n~~strike~~\n\nhttps://example.com/x\n\nTerm\n\n: def\n", opt)
	mustContain(t, out,
		`<table class="desk-table">`,
		`type="checkbox"`,
		`<del>strike</del>`,
		`href="https://example.com/x"`,
		`<dl>`, `<dt>Term</dt>`, `<dd>`, `def`,
	)
	mustContain(t, out, `<div class="desk-content">`)
}

func TestFootnotes(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "hi[^1]\n\n[^1]: the note\n", opt)
	mustContain(t, out, "the note", "fn:")
	mustLack(t, out, "<script")
}

func TestTOC(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "[toc]\n\n# Alpha\n\n## Beta\n", opt)
	mustContain(t, out,
		`<nav class="desk-toc"`,
		`href="#alpha"`, `href="#beta"`,
		`id="alpha"`, `id="beta"`,
	)
	// Without a marker no nav is emitted.
	plain := render(t, "# Solo\n", opt)
	mustLack(t, plain, "desk-toc")
}

func TestHighlighting(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "```go\npackage main\n\nfunc main() {}\n```\n", opt)
	mustContain(t, out,
		`<pre class="desk-code">`,
		`class="language-go"`,
		`desk-hl-`, // chroma class-based spans, no inline styles
	)
	mustLack(t, out, "style=")
	// Unknown language degrades to plain code.
	plain := render(t, "```nosuchlang\n<x>&\n```\n", opt)
	mustContain(t, plain, `<pre class="desk-code">`, `&lt;x&gt;&amp;`)
	mustLack(t, plain, "desk-hl-")
}

func TestCSVEmbed(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "![](data.csv)", opt)
	mustContain(t, out,
		`desk-embed-table`,
		`<table class="desk-table">`,
		`<th>name</th>`, `<th>age</th>`,
		`<td>ada</td>`, `<td>36</td>`,
		`class="desk-download"`,
		`href="/assets/x/data.csv"`,
	)
}

func TestCSVTruncation(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "![](big.csv)", opt)
	mustContain(t, out, "+5 more rows", `class="desk-download"`)
	if n := strings.Count(out, "<tr>"); n != MaxEmbedRows+1 {
		t.Errorf("want %d <tr> (header + %d rows), got %d", MaxEmbedRows+1, MaxEmbedRows, n)
	}
}

func TestTSVEmbed(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "![](data.tsv)", opt)
	mustContain(t, out, `desk-embed-table`, `<td>ada</td>`, `<td>36</td>`)
}

func TestImageEmbeds(t *testing.T) {
	_, opt := fixture(t)
	for _, tc := range []struct{ src, want string }{
		{"![](pic.png)", `<img class="desk-embed desk-embed-img" src="/assets/x/pic.png"`},
		{`![pic](pic.jpg)`, `<img class="desk-embed desk-embed-img" src="/assets/x/pic.jpg" alt="pic"`},
		{"![](pic.svg)", `<img class="desk-embed desk-embed-img" src="/assets/x/pic.svg"`},
	} {
		out := render(t, tc.src, opt)
		mustContain(t, out, tc.want)
	}
}

func TestVideoEmbeds(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "![](clip.mp4)\n\n![](clip.webm)", opt)
	mustContain(t, out,
		`<video class="desk-embed desk-embed-video" controls`,
		`src="/assets/x/clip.mp4"`,
		`src="/assets/x/clip.webm"`,
	)
}

func TestPDFEmbed(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "![](doc.pdf)", opt)
	mustContain(t, out,
		`desk-embed-pdf`,
		`<iframe src="/assets/x/doc.pdf"`,
		`class="desk-download"`,
	)
}

func TestChartFenceUnset(t *testing.T) {
	defer func() { ChartSVG = nil }()
	ChartSVG = nil
	_, opt := fixture(t)
	out := render(t, "```chart widget=bar\nname,value\na,1\nb,2\n```\n", opt)
	// No renderer wired: a code block plus an error note, never an SVG.
	mustContain(t, out, `desk-embed-chart`, "desk-embed-error",
		"chart renderer unavailable", `<pre class="desk-code">`)
	mustLack(t, out, "<svg")
}

func TestChartFenceHookError(t *testing.T) {
	defer func() { ChartSVG = nil }()
	ChartSVG = func(widget, format, title string, data []byte) (string, error) {
		return "", errTestChart
	}
	_, opt := fixture(t)
	out := render(t, "```chart widget=bogus\nnot data at all {{{\n```\n", opt)
	mustContain(t, out, "desk-embed-error")
	mustLack(t, out, "<svg")
}

func TestCSVFence(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "```csv\na,b\n1,2\n```\n", opt)
	mustContain(t, out, `desk-embed-table`, `<th>a</th>`, `<td>1</td>`)
	mustLack(t, out, "desk-download")
}

func TestMissingAndUnknownGraceful(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "![](nope.csv)", opt)
	mustContain(t, out, "desk-embed-missing", `href="/assets/x/nope.csv"`)
	out = render(t, "![](notes.txt)", opt)
	mustContain(t, out, `desk-embed-link`, `href="/assets/x/notes.txt"`)
}

func TestTraversalConfined(t *testing.T) {
	root := t.TempDir()
	secret := "SECRET-CANARY-12345"
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(root, "assets")
	if err := os.Mkdir(assets, 0700); err != nil {
		t.Fatal(err)
	}
	opt := Options{AssetDir: assets, AssetURL: "/assets/x/"}
	for _, src := range []string{"![](../secret.txt)", "![](../../secret.txt)", "![](/abs/secret.txt)", "![](..\\secret.txt)"} {
		out := render(t, src, opt)
		mustLack(t, out, secret, "secret.txt\" src=")
	}
}

func TestRemoteDefaultOff(t *testing.T) {
	_, opt := fixture(t)
	out := render(t, "![](https://evil.example/pic.png)\n\n![](https://evil.example/d.csv)\n", opt)
	mustContain(t, out, `<a class="desk-embed desk-embed-link" href="https://evil.example/pic.png"`)
	mustLack(t, out, "<img", "<video", "desk-embed-table")
}

func TestRemoteOptIn(t *testing.T) {
	_, opt := fixture(t)
	opt.AllowRemote = true
	out := render(t, "![](https://cdn.example/pic.png)\n\n![](https://cdn.example/clip.mp4)\n\n![](https://cdn.example/d.csv)\n", opt)
	mustContain(t, out,
		`<img class="desk-embed desk-embed-img" src="https://cdn.example/pic.png"`,
		`<video class="desk-embed desk-embed-video"`,
	)
	// Tabular remote stays a link: the server never fetches untrusted URLs.
	mustContain(t, out, `<a class="desk-embed desk-embed-link" href="https://cdn.example/d.csv"`)
}

func TestInjectionNeutralised(t *testing.T) {
	_, opt := fixture(t)
	src := "<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>\n\n[x](javascript:alert(1))\n\n![](javascript:alert(1))\n\n[evil](JaVaScRiPt:alert(1))\n"
	out := render(t, src, opt)
	// No executable residue: no script/style/iframe elements, no event
	// attributes, and no javascript: URL in any href/src. (The rejected
	// image destination may survive as inert escaped text, never a link.)
	mustLack(t, out,
		"<script", "onerror", "onclick", "<iframe", "<style",
		`href="javascript`, `href="JaVaScRiPt`, `src="javascript`,
	)
	mustContain(t, out, "<p>x</p>", "<p>evil</p>")
}

func TestInlineExport(t *testing.T) {
	_, opt := fixture(t)
	opt.Inline = true
	out := render(t, "![](pic.png)", opt)
	mustContain(t, out, `src="data:image/png;base64,`)
	mustLack(t, out, "<script")
}

func TestPage(t *testing.T) {
	body, err := Markdown([]byte("# Hi\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	page := string(Page(`A"B <C>`, body))
	mustContain(t, page,
		`<meta http-equiv="Content-Security-Policy"`,
		`default-src 'none'`,
		`frame-ancestors 'none'`,
		`<link rel="stylesheet" href="kit.css">`,
		`<script src="kit.js" defer></script>`,
		`<title>A&#34;B &lt;C&gt;</title>`,
	)
	mustLack(t, page,
		"<script>inline", "style=",
		`<form`, "http://", "https://",
	)
	// Exactly one script tag (kit JS only) and one stylesheet.
	if n := strings.Count(page, "<script"); n != 1 {
		t.Errorf("want 1 script tag, got %d", n)
	}
	if n := strings.Count(page, "<link"); n != 1 {
		t.Errorf("want 1 link tag, got %d", n)
	}
}

func TestChartHook(t *testing.T) {
	defer func() { ChartSVG = nil }()
	called := false
	ChartSVG = func(widget, format, title string, data []byte) (string, error) {
		called = true
		if widget != "line" || format != "json" {
			t.Errorf("hook got widget=%q format=%q", widget, format)
		}
		return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect class="desk-series-1" x="1" y="1" width="8" height="8"/><text class="desk-chart-label" x="1" y="9">a</text></svg>`, nil
	}
	_, opt := fixture(t)
	out := render(t, "```chart widget=line format=json\n[1]\n```\n", opt)
	if !called {
		t.Fatal("ChartSVG hook was not called")
	}
	// Hook SVG passes through, D1 palette classes intact, viewBox case kept.
	mustContain(t, out, "desk-embed-chart", "<svg", "viewBox",
		`class="desk-series-1"`, `class="desk-chart-label"`)
	mustLack(t, out, "viewbox=")
}
