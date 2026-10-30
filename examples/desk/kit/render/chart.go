package render

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"sort"
	"strconv"
	"strings"
)

// This file is the built-in ```chart fence renderer used while
// render.ChartSVG is unset. It accepts the same everyday input shapes as
// the desk chart tool (C1) — CSV with a header row, JSON arrays of numbers
// or of objects with numeric fields — and emits a server-side SVG with
// C1-style geometry. Presentation attributes only: no <style> element and
// no style attributes, so the output survives both the sanitizer and the
// strict view CSP. Styling hooks are the surrounding desk-embed-chart div
// (styled by kit.css, owned by S537.D1) and plain SVG descendant
// selectors.

type chartPoint struct {
	x     float64
	y     float64
	label string
}

type chartSeries struct {
	name   string
	points []chartPoint
}

// renderChartSVG is the ChartSVG-compatible signature used internally.
func renderChartSVG(widget, format, title string, data []byte) (string, error) {
	name := title
	series, table, err := parseChartData(data, format)
	if err != nil {
		return "", err
	}
	w := strings.ToLower(strings.TrimSpace(widget))
	switch w {
	case "", "auto", "line", "bar":
		if len(series) == 0 {
			return "", fmt.Errorf("chart needs numeric data")
		}
		if name == "" {
			name = series[0].name
		}
		return svgPlot(name, w, series[0])
	case "table":
		if table != nil {
			if name == "" {
				name = "Data"
			}
			return svgTable(name, table[0], table[1:])
		}
		if len(series) == 0 {
			return "", fmt.Errorf("chart needs data")
		}
		cols := []string{"#", series[0].name}
		rows := make([][]string, 0, len(series[0].points))
		for i, p := range series[0].points {
			rows = append(rows, []string{strconv.Itoa(i + 1), fmtNum(p.y)})
		}
		if name == "" {
			name = series[0].name
		}
		return svgTable(name, cols, rows)
	case "counter", "sum", "gauge", "stat":
		if len(series) == 0 {
			return "", fmt.Errorf("chart needs numeric data")
		}
		s := series[0]
		if name == "" {
			name = s.name
		}
		total := s.points[len(s.points)-1].y
		if w == "counter" || w == "sum" {
			total = 0
			for _, p := range s.points {
				total += p.y
			}
		}
		return svgStat(name, w, fmtNum(total), s)
	default:
		return "", fmt.Errorf("unknown widget %q", widget)
	}
}

// parseChartData returns numeric series for plotting plus, when the input
// is textual, a string table (header + rows). Numbers that are NaN/Inf
// are refused.
func parseChartData(raw []byte, format string) ([]chartSeries, [][]string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, nil, fmt.Errorf("chart data is empty")
	}
	if len(raw) > 256<<10 {
		return nil, nil, fmt.Errorf("chart data exceeds 256 KiB")
	}
	if format == "" || format == "auto" {
		format = "csv"
		if raw[0] == '{' || raw[0] == '[' {
			format = "json"
		}
	}
	switch strings.ToLower(format) {
	case "json", "ndjson":
		return chartFromJSON(raw)
	case "csv", "tsv":
		return chartFromCSV(raw, format)
	default:
		return nil, nil, fmt.Errorf("unknown chart format %q", format)
	}
}

func finite(f float64) (float64, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("non-finite number")
	}
	return f, nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		if f, err := finite(n); err == nil {
			return f, true
		}
	case json.Number:
		if f, err := n.Float64(); err == nil {
			if f, err := finite(f); err == nil {
				return f, true
			}
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
			if f, err := finite(f); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

// chartFromJSON accepts [1,2,3], [{"y":1},...] and {"series":[...]}-ish
// arrays; objects without numerics become a string table.
func chartFromJSON(raw []byte) ([]chartSeries, [][]string, error) {
	lines := [][]byte{raw}
	if trimmed := bytes.TrimSpace(raw); !bytes.HasPrefix(trimmed, []byte("[")) && !bytes.HasPrefix(trimmed, []byte("{")) {
		lines = bytes.Split(raw, []byte{'\n'})
	}
	var rows []any
	for _, ln := range lines {
		ln = bytes.TrimSpace(ln)
		if len(ln) == 0 {
			continue
		}
		var v any
		d := json.NewDecoder(bytes.NewReader(ln))
		d.UseNumber()
		if err := d.Decode(&v); err != nil {
			return nil, nil, err
		}
		if a, ok := v.([]any); ok {
			rows = append(rows, a...)
		} else {
			rows = append(rows, v)
		}
	}
	if len(rows) == 0 {
		return nil, nil, fmt.Errorf("chart data is empty")
	}
	cols := map[string]bool{}
	textual := false
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok {
			for k, v := range m {
				if k == "name" || k == "label" || k == "time" || k == "x" {
					continue
				}
				cols[k] = true
				if _, ok := toFloat(v); !ok {
					textual = true
				}
			}
		}
	}
	if textual {
		names := make([]string, 0, len(cols))
		for k := range cols {
			names = append(names, k)
		}
		sort.Strings(names)
		table := [][]string{names}
		for _, r := range rows {
			m, ok := r.(map[string]any)
			if !ok {
				return nil, nil, fmt.Errorf("table rows must be objects")
			}
			row := make([]string, 0, len(names))
			for _, k := range names {
				row = append(row, fmt.Sprint(m[k]))
			}
			table = append(table, row)
		}
		return nil, table, nil
	}
	byName := map[string]*chartSeries{}
	order := []string{}
	add := func(name string, p chartPoint) {
		s, ok := byName[name]
		if !ok {
			s = &chartSeries{name: name}
			byName[name] = s
			order = append(order, name)
		}
		s.points = append(s.points, p)
	}
	for i, r := range rows {
		label := strconv.Itoa(i + 1)
		x := float64(i)
		if m, ok := r.(map[string]any); ok {
			for _, k := range []string{"label", "time", "x"} {
				if z, ok := m[k]; ok {
					label = fmt.Sprint(z)
					if f, ok := toFloat(z); ok {
						x = f
					}
					break
				}
			}
			keys := []string{}
			for k := range m {
				if k != "label" && k != "time" && k != "x" && k != "name" {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			found := false
			for _, k := range keys {
				f, ok := toFloat(m[k])
				if !ok {
					return nil, nil, fmt.Errorf("column %s is not numeric", k)
				}
				name := k
				if s, ok := m["name"].(string); ok && k == "value" {
					name = s
				}
				add(name, chartPoint{x: x, y: f, label: label})
				found = true
			}
			if !found {
				return nil, nil, fmt.Errorf("row has no numeric columns")
			}
		} else {
			f, ok := toFloat(r)
			if !ok {
				return nil, nil, fmt.Errorf("value is not numeric")
			}
			add("value", chartPoint{x: x, y: f, label: label})
		}
		if len(order) > 0 && len(byName[order[0]].points) > 500 {
			break
		}
	}
	out := make([]chartSeries, 0, len(order))
	for _, k := range order {
		out = append(out, *byName[k])
	}
	return out, nil, nil
}

// chartFromCSV accepts header + rows; every numeric column becomes a
// series, and the raw grid doubles as the table widget's data.
func chartFromCSV(raw []byte, format string) ([]chartSeries, [][]string, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	if strings.ToLower(format) == "tsv" {
		r.Comma = '\t'
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	grid, err := r.ReadAll()
	if err != nil || len(grid) < 2 {
		return nil, nil, fmt.Errorf("chart CSV needs a header and data")
	}
	header := grid[0]
	numeric := make([]bool, len(header))
	for _, row := range grid[1:] {
		for i := range header {
			if i < len(row) {
				if _, ok := toFloat(strings.TrimSpace(row[i])); ok {
					numeric[i] = true
				}
			}
		}
	}
	var out []chartSeries
	for i, h := range header {
		if !numeric[i] {
			continue
		}
		s := chartSeries{name: h}
		for j, row := range grid[1:] {
			if i >= len(row) {
				continue
			}
			f, ok := toFloat(strings.TrimSpace(row[i]))
			if !ok {
				continue
			}
			s.points = append(s.points, chartPoint{x: float64(j), y: f, label: strconv.Itoa(j + 1)})
			if len(s.points) >= 500 {
				break
			}
		}
		if len(s.points) > 0 {
			out = append(out, s)
		}
	}
	return out, grid, nil
}

func esc(s string) string { return template.HTMLEscapeString(s) }

func fmtNum(v float64) string {
	if math.IsInf(v, 1) {
		return "+∞"
	}
	if math.IsInf(v, -1) {
		return "−∞"
	}
	return strconv.FormatFloat(v, 'g', 4, 64)
}

func svgHead(title string, height int) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" role="img" viewBox="0 0 640 ` +
		strconv.Itoa(height) + `" width="100%" aria-label="` + esc(title) + `">` +
		`<title>` + esc(title) + `</title>`
}

func svgText(x, y int, size int, muted bool, v string) string {
	fill := "currentColor"
	opacity := ""
	if muted {
		opacity = ` opacity="0.65"`
	}
	return fmt.Sprintf(`<text x="%d" y="%d" font-size="%d" font-family="system-ui,sans-serif" fill="%s"%s>%s</text>`,
		x, y, size, fill, opacity, esc(v))
}

// svgPlot draws one series as a line or bar plot with min/max labels.
func svgPlot(title, widget string, s chartSeries) (string, error) {
	if len(s.points) == 0 {
		return "", fmt.Errorf("chart needs numeric data")
	}
	var b strings.Builder
	b.WriteString(svgHead(title+" · "+widget, 300))
	b.WriteString(svgText(24, 32, 15, false, title))
	b.WriteString(svgText(24, 54, 12, true, widget))
	lo, hi := s.points[0].y, s.points[0].y
	for _, p := range s.points {
		lo = math.Min(lo, p.y)
		hi = math.Max(hi, p.y)
	}
	span := hi - lo
	if span == 0 {
		span = math.Abs(hi)
		if span == 0 {
			span = 1
		}
		lo -= span / 2
		hi += span / 2
		span = hi - lo
	}
	y := func(v float64) float64 { return 252 - (v-lo)/span*116 }
	n := len(s.points)
	x := func(i int) float64 {
		if n == 1 {
			return 320
		}
		return 24 + 592*float64(i)/float64(n-1)
	}
	b.WriteString(`<line x1="24" y1="252" x2="616" y2="252" stroke="currentColor" opacity="0.15"/>`)
	b.WriteString(svgText(24, 132, 12, true, fmtNum(hi)))
	b.WriteString(svgText(24, 274, 12, true, fmtNum(lo)))
	if widget == "bar" {
		width := 592 / float64(n)
		base := y(math.Min(0, lo))
		if lo >= 0 {
			base = y(lo)
		}
		for i, p := range s.points {
			py := y(p.y)
			top := math.Min(py, base)
			h := math.Abs(base - py)
			if h < 0.1 {
				h = 0.1
			}
			fmt.Fprintf(&b, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" rx="2" fill="currentColor" opacity="0.75"><title>%s: %s</title></rect>`,
				24+float64(i)*width, top, math.Max(0.1, width*0.8), h, esc(p.label), esc(fmtNum(p.y)))
		}
	} else {
		b.WriteString(`<polyline points="`)
		for i, p := range s.points {
			fmt.Fprintf(&b, `%.2f,%.2f `, x(i), y(p.y))
		}
		b.WriteString(`" fill="none" stroke="currentColor" stroke-width="3" stroke-linejoin="round" stroke-linecap="round"/>`)
		if n == 1 {
			fmt.Fprintf(&b, `<circle cx="320" cy="%.2f" r="4" fill="currentColor"/>`, y(s.points[0].y))
		}
	}
	b.WriteString(svgText(24, 294, 10, true, s.points[0].label))
	b.WriteString(`<text x="616" y="294" font-size="10" font-family="system-ui,sans-serif" fill="currentColor" opacity="0.65" text-anchor="end">` +
		esc(s.points[n-1].label) + `</text>`)
	b.WriteString(`</svg>`)
	return b.String(), nil
}

// svgStat renders counter/sum/gauge/stat as a headline number plus a
// sparkline of the series.
func svgStat(title, widget, total string, s chartSeries) (string, error) {
	var b strings.Builder
	b.WriteString(svgHead(title+" · "+widget, 300))
	b.WriteString(svgText(24, 32, 15, false, title))
	b.WriteString(svgText(24, 54, 12, true, widget))
	b.WriteString(svgText(24, 100, 28, false, total))
	lo, hi := s.points[0].y, s.points[0].y
	for _, p := range s.points {
		lo = math.Min(lo, p.y)
		hi = math.Max(hi, p.y)
	}
	span := hi - lo
	if span == 0 {
		span = 1
	}
	n := len(s.points)
	b.WriteString(`<polyline points="`)
	for i, p := range s.points {
		var px float64 = 320
		if n > 1 {
			px = 24 + 592*float64(i)/float64(n-1)
		}
		fmt.Fprintf(&b, `%.2f,%.2f `, px, 252-(p.y-lo)/span*116)
	}
	b.WriteString(`" fill="none" stroke="currentColor" stroke-width="3" stroke-linejoin="round" stroke-linecap="round"/>`)
	b.WriteString(`</svg>`)
	return b.String(), nil
}

// svgTable renders a string grid as SVG text rows (first row = header).
func svgTable(title string, header []string, rows [][]string) (string, error) {
	if len(header) == 0 {
		return "", fmt.Errorf("chart table needs columns")
	}
	if len(rows) > 12 {
		rows = rows[:12]
	}
	height := 120 + len(rows)*24
	var b strings.Builder
	b.WriteString(svgHead(title+" · table", height))
	b.WriteString(svgText(24, 32, 15, false, title))
	width := 592 / float64(len(header))
	for i, c := range header {
		b.WriteString(svgText(24+int(float64(i)*width), 86, 12, true, c))
	}
	for j, row := range rows {
		for i := range header {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			b.WriteString(svgText(24+int(float64(i)*width), 113+j*24, 12, false, cell))
		}
	}
	b.WriteString(`</svg>`)
	return b.String(), nil
}
