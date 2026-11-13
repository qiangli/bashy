// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// RSS writes the site feed from the newest posts.
func RSS(cfg SiteConfig, posts []*Post) string {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" || base == "." {
		base = "/"
	}

	var items strings.Builder
	limit := len(posts)
	if limit > 20 {
		limit = 20
	}
	for _, p := range posts[:limit] {
		link := fmt.Sprintf("%s/p/%s/", base, p.Slug)
		items.WriteString(fmt.Sprintf(`    <item>
      <title>%s</title>
      <link>%s</link>
      <guid>%s</guid>
      <pubDate>%s</pubDate>
      <description>%s</description>
    </item>
`, xmlEscape(p.Title), xmlEscape(link), xmlEscape(link), xmlEscape(rfc822(p.Date)), xmlEscape(p.Summary)))
	}

	joined := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>%s</title>
    <link>%s</link>
    <description>%s</description>
    <language>%s</language>
    <atom:link href="%s/index.xml" rel="self" type="application/rss+xml"/>
%s  </channel>
</rss>
`, xmlEscape(cfg.Title), xmlEscape(base), xmlEscape(cfg.Tagline), xmlEscape(cfg.Language), xmlEscape(base), items.String())
	return joined
}

func rfc822(date string) string {
	// YYYY-MM-DD (the format posts carry) to RFC 822: 2026-11-01 ->
	// Sun, 01 Nov 2026 00:00:00 +0000. Any other shape passes through.
	if len(date) == 10 && date[4] == '-' && date[7] == '-' {
		months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
		var y, m, d int
		if _, err := fmt.Sscanf(date, "%d-%d-%d", &y, &m, &d); err == nil && m >= 1 && m <= 12 {
			wd := weekday(y, m, d)
			return fmt.Sprintf("%s, %02d %s %d 00:00:00 +0000", wd, d, months[m-1], y)
		}
	}
	return date
}

func weekday(y, m, d int) string {
	if m < 3 {
		y--
		m += 12
	}
	k := y % 100
	j := y / 100
	h := (d + 13*(m+1)/5 + k + k/4 + j/4 + 5*j) % 7
	names := []string{"Sat", "Sun", "Mon", "Tue", "Wed", "Thu", "Fri"}
	return names[h]
}

func xmlEscape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}
