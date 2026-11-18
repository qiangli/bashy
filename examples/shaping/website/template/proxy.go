package main

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// upstream fronts the existing public site: the engine face serves the very
// pages the static host serves, so the site itself needs no change. Pages are
// cached briefly; HTML gets root-absolute links re-based under the mount
// prefix and the shaping panel injected before </body>.
type upstream struct {
	base   string // e.g. https://example.com (no trailing slash)
	client *http.Client
	ttl    time.Duration

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	status int
	ctype  string
	body   []byte
	at     time.Time
}

func newUpstream(base string) *upstream {
	return &upstream{
		base:   strings.TrimRight(base, "/"),
		client: &http.Client{Timeout: 20 * time.Second},
		ttl:    5 * time.Minute,
		cache:  map[string]cached{},
	}
}

func (u *upstream) fetch(p string) (cached, error) {
	u.mu.Lock()
	c, ok := u.cache[p]
	u.mu.Unlock()
	if ok && time.Since(c.at) < u.ttl {
		return c, nil
	}
	resp, err := u.client.Get(u.base + p)
	if err != nil {
		return cached{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return cached{}, err
	}
	c = cached{status: resp.StatusCode, ctype: resp.Header.Get("Content-Type"), body: body, at: time.Now()}
	if resp.StatusCode == http.StatusOK {
		u.mu.Lock()
		u.cache[p] = c
		u.mu.Unlock()
	}
	return c, nil
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "read-only", http.StatusMethodNotAllowed)
		return
	}
	p := r.URL.EscapedPath()
	if r.URL.RawQuery != "" {
		p += "?" + r.URL.RawQuery
	}
	c, err := u.fetch(p)
	if err != nil {
		http.Error(w, "site unreachable", http.StatusBadGateway)
		return
	}
	prefix := r.Header.Get("X-Forwarded-Prefix")
	body := c.body
	switch {
	case strings.Contains(c.ctype, "text/html"):
		body = injectPanel(rebase(body, prefix), prefix)
	case strings.Contains(c.ctype, "css"), strings.Contains(c.ctype, "javascript"):
		body = rebase(body, prefix)
	}
	if c.ctype != "" {
		w.Header().Set("Content-Type", c.ctype)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(c.status)
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}

var (
	attrRoot = regexp.MustCompile(`((?:href|src|action|poster|content)=["'])/([^/])`)
	urlRoot  = regexp.MustCompile(`url\((["']?)/([^/])`)
	nextRoot = regexp.MustCompile(`(\\?["'])/_next/`)
)

// rebase points root-absolute links at the mount prefix, so a page built for
// "/" works under /matrix/h/<host>/app/<name>/. Protocol-relative links
// ("//cdn…") are left alone.
func rebase(b []byte, prefix string) []byte {
	if prefix == "" {
		return b
	}
	b = attrRoot.ReplaceAll(b, []byte("${1}"+prefix+"/${2}"))
	b = urlRoot.ReplaceAll(b, []byte("url(${1}"+prefix+"/${2}"))
	b = nextRoot.ReplaceAll(b, []byte("${1}"+prefix+"/_next/"))
	return b
}

func injectPanel(b []byte, prefix string) []byte {
	tag := []byte(`<link rel="stylesheet" href="` + prefix + `/_shaping/shaping.css"><script src="` + prefix + `/_shaping/shaping.js" defer></script>`)
	if i := bytes.LastIndex(b, []byte("</body>")); i >= 0 {
		out := make([]byte, 0, len(b)+len(tag))
		out = append(out, b[:i]...)
		out = append(out, tag...)
		return append(out, b[i:]...)
	}
	return append(b, tag...)
}

var tagRe = regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<[^>]+>`)

// pageText is the public page reduced to text, the public half of what a
// view is generated from.
func pageText(html []byte, limit int) string {
	t := tagRe.ReplaceAllString(string(html), " ")
	t = strings.Join(strings.Fields(t), " ")
	if len(t) > limit {
		t = t[:limit]
	}
	return t
}
