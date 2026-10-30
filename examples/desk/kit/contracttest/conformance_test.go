package contracttest

import (
	"strings"
	"testing"

	kit "desk-kit"
)

func page(tool string) string {
	return string(kit.Page("probe", "probe", kit.AppBar("probe", "/", kit.EmptyItemHeader("probe"), "")+tool))
}

func TestStyleViolationsGreen(t *testing.T) {
	clean := page(`<style>#message:empty{display:none}#add,#facade{padding:var(--desk-space-2)}
.x{color:var(--desk-ink);font:inherit}.y{font-family:var(--desk-font-mono)}
@media(max-width:650px){.z{background:var(--desk-subtle)}}</style>
<div style="margin-top:10px"></div><svg><rect fill="#2563eb"/></svg>
<script>document.querySelector('#add');ctx.strokeStyle=pen.value||'#ff0000';</script>`)
	if v := StyleViolations(clean, []string{"#ff0000"}); len(v) != 0 {
		t.Fatalf("clean page: want no violations, got %v", v)
	}
}

func TestStyleViolationsRed(t *testing.T) {
	for name, tool := range map[string]string{
		"hex in style block":    `<style>.a{color:#c0ffee}</style>`,
		"hex in media block":    `<style>@media(prefers-color-scheme:dark){.a{background:#111}}</style>`,
		"rgba in style block":   `<style>.a{box-shadow:0 1px 2px rgba(0,0,0,.2)}</style>`,
		"font-family":           `<style>.a{font-family:Georgia,serif}</style>`,
		"font shorthand":        `<style>.a{font:14px/1.4 system-ui}</style>`,
		"style attribute":       `<p style="color:#e00">x</p>`,
		"svg fill":              `<svg><rect fill="#123456"/></svg>`,
		"svg font attribute":    `<svg><text font-family="Arial">x</text></svg>`,
		"script colour":         `<script>el.style.color = bad ? '#e00' : 'inherit';</script>`,
		"script canvas colour":  `<script>ctx.fillStyle='#fff';</script>`,
		"script font":           `<script>ctx.font='12px sans-serif';</script>`,
		"script innerHTML attr": `<script>x.innerHTML='<b style="color:#abc">';</script>`,
	} {
		if v := StyleViolations(page(tool), nil); len(v) == 0 {
			t.Errorf("%s: planted violation stayed green", name)
		}
	}
	noKit := `<!doctype html><html><head><style>.a{color:var(--desk-ink)}</style></head><body></body></html>`
	v := StyleViolations(noKit, nil)
	if len(v) != 2 || !strings.Contains(strings.Join(v, "\n"), "kit.css") || !strings.Contains(strings.Join(v, "\n"), "app shell") {
		t.Errorf("page without kit: want kit.css + shell violations, got %v", v)
	}
}

func TestEveryFamilyToolHasAnAccent(t *testing.T) {
	for _, tool := range kit.Tools {
		if !strings.Contains(kit.CSS(), `:root[data-desk-tool="`+tool.Name+`"] { --desk-accent: var(--desk-accent-`+tool.Name+`); }`) {
			t.Errorf("tool %s: no family accent selector in kit.css", tool.Name)
		}
		if strings.Count(kit.CSS(), "--desk-accent-"+tool.Name+":") != 3 {
			t.Errorf("tool %s: accent must be defined for light, prefers-dark and [data-theme=dark]", tool.Name)
		}
	}
	for _, c := range kit.DataPalette {
		if !strings.Contains(kit.CSS(), c+";") {
			t.Errorf("data palette colour %s missing from kit.css --desk-data-*", c)
		}
	}
}
