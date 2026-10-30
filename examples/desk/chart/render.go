package main

import (
	"bytes"
	"embed"
	"html/template"

	"desk-chart/fence"
)

//go:embed templates/*.html
var templateFiles embed.FS

// SVG geometry lives in fence (shared with kit ```chart fences); the
// wrappers below keep main-package call sites unchanged.
var widgetKinds = fence.WidgetKinds

func validWidget(s string) bool { return fence.ValidWidget(s) }

func renderSVG(s series, widget string) (string, error) {
	return fence.RenderSVG(s, widget)
}

func esc(s string) string { return template.HTMLEscapeString(s) }

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
