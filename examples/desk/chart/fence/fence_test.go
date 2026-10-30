package fence

import (
	"strings"
	"testing"
)

func mustHave(t *testing.T, svg string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(svg, w) {
			t.Errorf("SVG lacks %q:\n%s", w, svg)
		}
	}
}

func mustLack(t *testing.T, svg string, banned ...string) {
	t.Helper()
	for _, b := range banned {
		if strings.Contains(svg, b) {
			t.Errorf("SVG contains banned %q:\n%s", b, svg)
		}
	}
}

// The fence renderer reuses the C1 parser and geometry and its output must
// stay sanitizer-safe: D1 palette classes, no <style>, no style or
// font-family attributes.
func TestRenderFenceShapes(t *testing.T) {
	for _, tc := range []struct {
		name, widget, format, title, data string
		want                              []string
	}{
		{"csv-bar", "bar", "", "", "name,value\na,1\nb,2\n", []string{"<svg", "viewBox", "desk-series-1"}},
		{"json-line", "line", "json", "T", "[1,2,3]\n", []string{"<svg", "desk-chart-trace", "T · line"}},
		{"tsv", "bar", "tsv", "", "name\tvalue\na\t1\n", []string{"<svg", "desk-series-1"}},
		{"table", "table", "", "", "a,b\n1,2\n", []string{"<svg", "table"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svg, err := RenderFence(tc.widget, tc.format, tc.title, []byte(tc.data))
			if err != nil {
				t.Fatalf("RenderFence: %v", err)
			}
			mustHave(t, svg, tc.want...)
			mustLack(t, svg, "<style", "style=", "font-family")
		})
	}
}

func TestRenderFenceErrors(t *testing.T) {
	for _, data := range []string{"", "   \n", "name,value\na,b\n", "```not csv at all {{{"} {
		if _, err := RenderFence("bar", "", "", []byte(data)); err == nil {
			t.Errorf("accepted %q", data)
		}
	}
	if _, err := RenderFence("bogus", "", "", []byte("a,b\n1,2\n")); err == nil {
		t.Error("accepted unknown widget")
	}
	if _, err := RenderFence("bar", "", "", []byte("a,b\n1,2\n"+strings.Repeat("x", 256<<10))); err == nil {
		t.Error("accepted oversized data")
	}
}
