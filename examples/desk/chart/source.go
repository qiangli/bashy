package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type chartConfig struct {
	Input    string    `json:"input"`
	Format   string    `json:"format,omitempty"`
	Widget   string    `json:"widget,omitempty"`
	Title    string    `json:"title,omitempty"`
	Template string    `json:"template,omitempty"`
	Source   string    `json:"source,omitempty"`
	Refresh  string    `json:"refresh,omitempty"`
	Updated  time.Time `json:"updated,omitempty"`
}

func readBounded(r io.Reader) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, maxInput+1))
	if e != nil {
		return nil, e
	}
	if len(b) > maxInput {
		return nil, fmt.Errorf("input exceeds 1 MiB")
	}
	return b, nil
}
func checkSource(s string) error {
	if strings.Contains(s, "://") {
		u, e := url.Parse(s)
		if e != nil {
			return e
		}
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || ip == nil || !ip.IsLoopback() || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("refresh URL must be HTTP with a literal loopback IP and no credentials or fragment")
		}
		return nil
	}
	if !filepath.IsAbs(s) {
		return fmt.Errorf("refresh file must be an absolute path")
	}
	return nil
}
func readSource(s string) ([]byte, error) {
	if e := checkSource(s); e != nil {
		return nil, e
	}
	if strings.Contains(s, "://") {
		client := http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("refresh redirects refused") }}
		res, e := client.Get(s)
		if e != nil {
			return nil, e
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("refresh source returned HTTP %d", res.StatusCode)
		}
		return readBounded(res.Body)
	}
	info, e := os.Stat(s)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source must be a regular file")
	}
	f, e := os.Open(s)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	info, e = f.Stat()
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source must be a regular file")
	}
	return readBounded(f)
}
func prepareConfig(c *chartConfig) error {
	if len(c.Template) > maxInput {
		return fmt.Errorf("template exceeds 1 MiB")
	}
	c.Widget = strings.ToLower(c.Widget)
	if !validWidget(c.Widget) {
		return fmt.Errorf("unknown widget %q", c.Widget)
	}
	if c.Refresh != "" {
		d, e := time.ParseDuration(c.Refresh)
		if e != nil || d < time.Second || d > 24*time.Hour {
			return fmt.Errorf("refresh must be between 1s and 24h")
		}
		if c.Source == "" {
			return fmt.Errorf("refresh requires a file or loopback URL")
		}
		if e = checkSource(c.Source); e != nil {
			return e
		}
	}
	if c.Source != "" {
		raw, e := readSource(c.Source)
		if e != nil {
			return e
		}
		c.Input = string(raw)
	}
	if _, e := renderCharts(*c); e != nil {
		return e
	}
	c.Updated = time.Now().UTC()
	return nil
}
