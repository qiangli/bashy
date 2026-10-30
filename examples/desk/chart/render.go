package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"math"
	"strings"
)

//go:embed templates/*.html
var templateFiles embed.FS

var widgetKinds = []string{"counter", "sum", "gauge", "updowncounter", "histogram", "exponentialhistogram", "line", "bar", "table", "stat"}

func validWidget(s string) bool {
	if s == "" || s == "auto" {
		return true
	}
	for _, k := range widgetKinds {
		if k == s {
			return true
		}
	}
	return false
}
func fmtNum(v float64) string {
	if math.IsInf(v, 1) {
		return "+∞"
	}
	if math.IsInf(v, -1) {
		return "−∞"
	}
	return fmt.Sprintf("%.4g", v)
}
func esc(s string) string { return template.HTMLEscapeString(s) }

// All geometry is generated here; templates control the surrounding page only.
func renderSVG(s series, widget string) (string, error) {
	if widget == "" || widget == "auto" {
		widget = s.Kind
	}
	if !validWidget(widget) {
		return "", fmt.Errorf("unknown widget %q", widget)
	}
	if (widget == "histogram" || widget == "exponentialhistogram") && len(s.Buckets) == 0 && s.Kind != "histogram" && s.Kind != "exponentialhistogram" {
		return "", fmt.Errorf("distribution widget requires histogram buckets")
	}
	if widget != "histogram" && widget != "exponentialhistogram" && len(s.Points) == 0 && !(widget == "table" && len(s.Rows) > 0) {
		return "", fmt.Errorf("%s requires numeric points", widget)
	}
	height := 300
	if widget == "table" {
		n := len(s.Points)
		if len(s.Rows) > 0 {
			n = len(s.Rows)
		}
		if n > 7 {
			height = 120 + n*24
		}
	}
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" role="img" viewBox="0 0 640 ` + fmt.Sprint(height) + `" style="width:100%;color:var(--desk-ink);background:var(--desk-panel);border-radius:var(--desk-radius-lg,12px)" aria-label="` + esc(s.Name+" · "+widget) + `"><title>` + esc(s.Name+" · "+widget) + `</title><style>text{font-family:var(--desk-font,system-ui,sans-serif);fill:currentColor}.muted{opacity:.65}.stat{font-size:28px;font-weight:650}.trace{fill:none;stroke:var(--desk-data-1,#2563eb);stroke-width:3;stroke-linejoin:round;stroke-linecap:round}.bar{fill:var(--desk-data-1,#2563eb)}</style>`)
	text := func(x, y int, class, value string) {
		fmt.Fprintf(&b, `<text x="%d" y="%d" class="%s">%s</text>`, x, y, class, esc(value))
	}
	name := s.Name
	if len([]rune(name)) > 72 {
		name = string([]rune(name)[:69]) + "…"
	}
	text(24, 32, "", name)
	text(24, 54, "muted", widget+" · "+s.Unit)
	points := s.Points
	switch widget {
	case "counter", "sum":
		total := points[len(points)-1].Y
		if s.Delta {
			total = 0
			for _, p := range points {
				total += p.Y
			}
		}
		text(24, 94, "stat", fmtNum(total)+" total")
		text(24, 116, "muted", "Rate / second")
		points = ratePoints(s)
	case "gauge", "stat":
		text(24, 100, "stat", fmtNum(points[len(points)-1].Y)+" "+s.Unit)
	case "histogram", "exponentialhistogram":
		for i, q := range []float64{.5, .95, .99} {
			fmt.Fprintf(&b, `<rect x="%d" y="66" width="190" height="36" rx="8" fill="currentColor" opacity=".06"/>`, 16+i*208)
			text(24+i*208, 90, "", fmt.Sprintf("p%d %s", int(q*100), quantile(s.Buckets, q)))
		}
		text(24, 113, "muted", "Latest distribution · estimated percentiles")
		points = nil
		for i, v := range s.Buckets {
			points = append(points, point{X: float64(i), Y: v.Count, Label: fmtNum(v.Low) + "…" + fmtNum(v.High)})
		}
	}

	if widget == "table" {
		if len(s.Rows) > 0 {
			width := 592 / float64(len(s.Columns))
			for i, c := range s.Columns {
				text(24+int(float64(i)*width), 86, "muted", c)
			}
			for j, row := range s.Rows {
				for i, c := range row {
					text(24+int(float64(i)*width), 113+j*24, "", c)
				}
			}
		} else {
			text(24, 86, "muted", "Time / label")
			text(460, 86, "muted", "Value")
			for i, p := range points {
				text(24, 113+i*24, "", p.Label)
				text(460, 113+i*24, "", fmtNum(p.Y))
			}
		}

	} else if widget != "stat" {
		if len(points) == 0 {
			if widget == "histogram" || widget == "exponentialhistogram" {
				text(24, 190, "muted", "No observations")
			} else {
				text(24, 190, "muted", "Need another timed sample to calculate a rate")
			}
		} else {
			drawPlot(&b, points, widget == "bar" || widget == "histogram" || widget == "exponentialhistogram")
		}
	}
	b.WriteString(`</svg>`)
	return b.String(), nil
}
func drawPlot(b *strings.Builder, ps []point, bars bool) {
	lo, hi := ps[0].Y, ps[0].Y
	for _, p := range ps {
		lo = math.Min(lo, p.Y)
		hi = math.Max(hi, p.Y)
	}
	if bars {
		lo = math.Min(0, lo)
		hi = math.Max(0, hi)
	}

	// Normalize first so constant values near MaxFloat64 cannot collapse the
	// range or overflow when padded.
	scale := math.Max(math.Abs(lo), math.Abs(hi))
	if scale == 0 {
		scale = 1
	}
	low, high := lo/scale, hi/scale
	if low == high {
		low -= .5
		high += .5
	}
	y := func(v float64) float64 { return 252 - (v/scale-low)/(high-low)*116 }
	xmin, xmax := ps[0].X, ps[len(ps)-1].X
	x := func(i int) float64 {
		if xmax == xmin {
			return 320
		}
		if ps[0].Nano != 0 && ps[len(ps)-1].Nano > ps[0].Nano {
			return 24 + 592*float64(ps[i].Nano-ps[0].Nano)/float64(ps[len(ps)-1].Nano-ps[0].Nano)
		}
		xs := math.Max(math.Abs(xmin), math.Abs(xmax))
		return 24 + 592*(ps[i].X/xs-xmin/xs)/(xmax/xs-xmin/xs)
	}
	fmt.Fprintf(b, `<path d="M24 252H616" stroke="currentColor" opacity=".15"/><text x="24" y="132" class="muted">%s</text><text x="24" y="274" class="muted">%s</text>`, esc(fmtNum(hi)), esc(fmtNum(lo)))
	if bars {
		width := 592 / float64(len(ps))
		base := y(0)
		for i, p := range ps {
			py := y(p.Y)
			fmt.Fprintf(b, `<rect class="bar" x="%.2f" y="%.2f" width="%.2f" height="%.2f" rx="2"><title>%s: %s</title></rect>`, 24+float64(i)*width, math.Min(py, base), math.Max(.1, width*.8), math.Abs(base-py), esc(p.Label), esc(fmtNum(p.Y)))
		}
	} else {
		b.WriteString(`<polyline class="trace" points="`)
		for i, p := range ps {
			fmt.Fprintf(b, "%.2f,%.2f ", x(i), y(p.Y))
		}
		b.WriteString(`"/>`)
		if len(ps) == 1 {
			fmt.Fprintf(b, `<circle cx="320" cy="%.2f" r="4" class="bar"/>`, y(ps[0].Y))
		}
	}
	first, last := ps[0].Label, ps[len(ps)-1].Label
	fmt.Fprintf(b, `<text x="24" y="294" class="muted" font-size="10">%s</text><text x="616" y="294" text-anchor="end" class="muted" font-size="10">%s</text>`, esc(first), esc(last))
}

type templateView struct {
	Title, Description string
	Widgets            []template.HTML
}

func renderCharts(c chartConfig) (string, error) {
	ss, e := parseData([]byte(c.Input), c.Format)
	if e != nil {
		return "", e
	}
	view := templateView{Title: c.Title, Description: "A snapshot of your numbers."}
	if c.Refresh != "" {
		view.Description = "Refreshes every " + c.Refresh + " while viewed."
	}
	for _, s := range ss {
		svg, e := renderSVG(s, c.Widget)
		if e != nil {
			return "", e
		}
		view.Widgets = append(view.Widgets, template.HTML(svg))
	}
	source := c.Template
	if source == "" {
		b, _ := templateFiles.ReadFile("templates/page.html")
		source = string(b)
	}
	t, e := template.New("chart").Parse(source)
	if e != nil {
		return "", e
	}
	var b bytes.Buffer
	if e = t.Execute(&b, view); e != nil {
		return "", e
	}
	return b.String(), nil
}
