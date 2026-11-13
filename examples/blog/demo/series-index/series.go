// Copyright (c) 2026 qiangli
// See LICENSE for licensing information
//
// The demo wish: a series index on the home page. The base journal links
// posts that share a series but the home page has no series overview; this
// candidate adds one — a section above the post list naming each series
// and its posts in reading order. It is the entire change: one extra file
// in generator/, no base file edited, and the set of published routes is
// untouched (the section lives in the existing public/index.html), so the
// change can be verified, approved for one principal, and undone.
package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func init() {
	seriesIndexHook = renderSeriesIndexes
}

func renderSeriesIndexes(cfg SiteConfig, posts []*Post, outDir string) error {
	bySeries := map[string][]*Post{}
	var names []string
	for _, p := range posts {
		if p.Series == "" {
			continue
		}
		if _, seen := bySeries[p.Series]; !seen {
			names = append(names, p.Series)
		}
		bySeries[p.Series] = append(bySeries[p.Series], p)
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)

	var items strings.Builder
	for _, name := range names {
		sib := bySeries[name]
		sort.Slice(sib, func(i, j int) bool {
			if sib[i].Part != sib[j].Part && sib[i].Part != "" && sib[j].Part != "" {
				return sib[i].Part < sib[j].Part
			}
			return sib[i].Date < sib[j].Date
		})
		var links strings.Builder
		for i, p := range sib {
			if i > 0 {
				links.WriteString(" &middot; ")
			}
			part := ""
			if p.Part != "" {
				part = " (" + html.EscapeString(p.Part) + ")"
			}
			fmt.Fprintf(&links, `<a href="p/%s/">%s</a>%s`, p.Slug, html.EscapeString(p.Title), part)
		}
		fmt.Fprintf(&items, `<li><strong>%s</strong> — %s</li>`, html.EscapeString(name), links.String())
	}

	section := fmt.Sprintf(`<section class="series-list">
<h2>Series</h2>
<ul>
%s
</ul>
</section>
`, items.String())

	home := filepath.Join(outDir, "index.html")
	data, err := os.ReadFile(home)
	if err != nil {
		return err
	}
	page := string(data)
	const anchor = `<section class="post-list">`
	if !strings.Contains(page, anchor) {
		return fmt.Errorf("series index: home page anchor %q not found", anchor)
	}
	page = strings.Replace(page, anchor, section+anchor, 1)
	return os.WriteFile(home, []byte(page), 0o644)
}
