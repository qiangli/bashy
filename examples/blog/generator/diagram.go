// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"fmt"
	"strings"
)

// DiagramSVG renders a small text diagram to inline SVG. The input is an
// edge list, one edge per line:
//
//	source -> target
//	source .> target : dashed, with a label
//	source = target  : double line
//
// Node ids become boxes; an id written as kebab-case or snake_case is
// humanized for the label. Layout is layered left to right by reachability
// depth, so a pipeline reads like its steps.
func DiagramSVG(src string) (string, error) {
	type edge struct {
		from, to, label, style string
	}
	var edges []edge
	nodes := map[string]bool{}
	nodeOrder := []string{}

	addNode := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if !nodes[id] {
			nodes[id] = true
			nodeOrder = append(nodeOrder, id)
		}
	}

	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		style := "solid"
		op := ""
		for _, candidate := range []string{"->", ".>", "=>", "--"} {
			if strings.Contains(line, candidate) {
				op = candidate
				break
			}
		}
		if op == "" {
			return "", fmt.Errorf("diagram: not an edge: %q", line)
		}
		parts := strings.SplitN(line, op, 2)
		from := strings.TrimSpace(parts[0])
		rest := strings.TrimSpace(parts[1])
		label := ""
		if i := strings.Index(rest, ":"); i >= 0 {
			label = strings.TrimSpace(rest[i+1:])
			rest = strings.TrimSpace(rest[:i])
		}
		to := strings.TrimSpace(rest)
		if from == "" || to == "" {
			return "", fmt.Errorf("diagram: edge needs a source and a target: %q", line)
		}
		switch op {
		case ".>":
			style = "dashed"
		case "=>":
			style = "thick"
		case "--":
			style = "plain"
		}
		addNode(from)
		addNode(to)
		edges = append(edges, edge{from: from, to: to, label: label, style: style})
	}

	if len(nodes) == 0 {
		return "", fmt.Errorf("diagram: empty")
	}

	// Layer by longest distance from any root; cycles fall into later
	// layers and still render.
	depth := map[string]int{}
	for _, id := range nodeOrder {
		depth[id] = 0
	}
	for changed := true; changed; {
		changed = false
		for _, e := range edges {
			if depth[e.to] < depth[e.from]+1 {
				depth[e.to] = depth[e.from] + 1
				changed = true
			}
		}
	}

	layers := map[int][]string{}
	maxDepth := 0
	for _, id := range nodeOrder {
		d := depth[id]
		if d > 40 {
			return "", fmt.Errorf("diagram: too deep (cycle?)")
		}
		layers[d] = append(layers[d], id)
		if d > maxDepth {
			maxDepth = d
		}
	}

	const (
		boxW     = 150
		boxH     = 38
		gapX     = 70
		gapY     = 22
		pad      = 14
		maxRows  = 24
		fontSize = 13
	)

	rows := 0
	for d := 0; d <= maxDepth; d++ {
		if len(layers[d]) > rows {
			rows = len(layers[d])
		}
	}
	if rows > maxRows {
		return "", fmt.Errorf("diagram: too many nodes in one column (%d)", rows)
	}

	width := pad*2 + (maxDepth+1)*boxW + maxDepth*gapX
	height := pad*2 + rows*boxH + (rows-1)*gapY

	pos := map[string][2]float64{}
	for d := 0; d <= maxDepth; d++ {
		for r, id := range layers[d] {
			pos[id] = [2]float64{pad + float64(d)*(boxW+gapX), pad + float64(r)*(boxH+gapY)}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="diagram-svg" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" role="img" aria-label="diagram">`, width, height)
	fmt.Fprintf(&b, `<defs><marker id="dg-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 1 L 9 5 L 0 9 z" class="dg-arrowhead"/></marker></defs>`)

	for _, e := range edges {
		x1, y1 := pos[e.from][0]+boxW, pos[e.from][1]+boxH/2
		x2, y2 := pos[e.to][0], pos[e.to][1]+boxH/2
		cls := "dg-edge"
		switch e.style {
		case "dashed":
			cls += " dg-dashed"
		case "thick":
			cls += " dg-thick"
		}
		mx, my := (x1+x2)/2, (y1+y2)/2
		path := fmt.Sprintf("M %g %g L %g %g", x1, y1, x2, y2)
		if e.from == e.to {
			path = fmt.Sprintf("M %g %g C %g %g, %g %g, %g %g", x1, y1, x1+34, y1-34, x2-34, y2-34, x2, y2)
			mx, my = x1+34, y1-40
		}
		fmt.Fprintf(&b, `<path class="%s" d="%s" marker-end="url(#dg-arrow)"/>`, cls, path)
		if e.label != "" {
			fmt.Fprintf(&b, `<text class="dg-edge-label" x="%g" y="%g" text-anchor="middle">%s</text>`, mx, my-5, escapeSVG(e.label))
		}
	}

	for _, id := range nodeOrder {
		x, y := pos[id][0], pos[id][1]
		fmt.Fprintf(&b, `<g class="dg-node"><rect x="%g" y="%g" width="%d" height="%d" rx="7"/><text x="%g" y="%g" text-anchor="middle">%s</text></g>`, x, y, boxW, boxH, x+boxW/2, y+boxH/2+5, escapeSVG(humanize(id)))
	}

	b.WriteString(`</svg>`)
	return b.String(), nil
}

func humanize(id string) string {
	id = strings.NewReplacer("_", " ", "-", " ").Replace(id)
	words := strings.Fields(id)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	out := strings.Join(words, " ")
	if len(out) > 22 {
		out = out[:21] + "…"
	}
	return out
}

func escapeSVG(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}
