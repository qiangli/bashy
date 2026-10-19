// Copyright (c) 2026 Qiang Li. All rights reserved.
// Ported from kg-ui under explicit operator authorization for Bashy.
// See ATTRIBUTION.md and LICENSE for licensing information.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTestServer(t *testing.T) (*httptest.Server, *BashyCLIAdapter, *SSEHub, string) {
	t.Helper()

	root := t.TempDir()
	docPath := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(docPath, []byte("knowledge document contents"), 0644); err != nil {
		t.Fatal(err)
	}

	binPath := filepath.Join(root, "binary.bin")
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}, 0644); err != nil {
		t.Fatal(err)
	}

	rm := NewRootsManager([]string{root})
	adapter := NewBashyCLIAdapter("bashy", rm)
	hub := NewSSEHub()

	handler, err := buildMux(adapter, hub)
	if err != nil {
		t.Fatalf("buildMux: %v", err)
	}

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return srv, adapter, hub, root
}

func TestServerHealthAndStatic(t *testing.T) {
	srv, _, _, _ := setupTestServer(t)

	// 1. /healthz
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"ok":true`) {
		t.Errorf("want ok:true, got %s", string(body))
	}

	// 2. / (index.html)
	resp, err = http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `data-tab="explore"`) {
		t.Errorf("missing explore tab in index.html")
	}

	// 3. /styles.css
	resp, err = http.Get(srv.URL + "/styles.css")
	if err != nil {
		t.Fatalf("GET /styles.css: %v", err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "--bg") {
		t.Errorf("missing --bg in styles.css")
	}
}

func TestServerSSEStream(t *testing.T) {
	srv, _, _, _ := setupTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE status: want 200, got %d", resp.StatusCode)
	}

	sc := bufio.NewScanner(resp.Body)
	foundInit := false
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, "init") {
			foundInit = true
			break
		}
	}
	if !foundInit {
		t.Errorf("did not receive initial SSE event")
	}
}

func TestServerFilePreviewSecurity(t *testing.T) {
	srv, _, _, root := setupTestServer(t)

	samplePath := filepath.Join(root, "sample.txt")
	binaryPath := filepath.Join(root, "binary.bin")

	// 1. Valid file preview
	resp, err := http.Get(srv.URL + "/api/file?uri=" + samplePath)
	if err != nil {
		t.Fatalf("GET /api/file: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", resp.StatusCode, string(body))
	}
	if string(body) != "knowledge document contents" {
		t.Errorf("unexpected body: %s", string(body))
	}
	if resp.Header.Get("X-Kg-Bytes") == "" || resp.Header.Get("X-Kg-Sha256") == "" {
		t.Errorf("missing X-Kg headers")
	}

	// 2. Binary file preview returns 415
	resp, err = http.Get(srv.URL + "/api/file?uri=" + binaryPath)
	if err != nil {
		t.Fatalf("GET /api/file binary: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("binary preview: want 415, got %d", resp.StatusCode)
	}

	// 3. Binary file with download=1 returns 200 and attachment header
	resp, err = http.Get(srv.URL + "/api/file?uri=" + binaryPath + "&download=1")
	if err != nil {
		t.Fatalf("GET /api/file binary download: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("binary download: want 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Errorf("missing attachment header on download")
	}

	// 4. Outside-root file preview returns 403 Forbidden
	outsideFile := filepath.Join(t.TempDir(), "outside.txt")
	_ = os.WriteFile(outsideFile, []byte("outside"), 0644)
	resp, err = http.Get(srv.URL + "/api/file?uri=" + outsideFile)
	if err != nil {
		t.Fatalf("GET /api/file outside: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("outside-root: want 403, got %d", resp.StatusCode)
	}
}

func TestServerRootsEndpoints(t *testing.T) {
	srv, _, _, root := setupTestServer(t)

	// 1. GET /api/roots
	resp, err := http.Get(srv.URL + "/api/roots")
	if err != nil {
		t.Fatalf("GET /api/roots: %v", err)
	}
	defer resp.Body.Close()
	var listResp struct {
		Roots []RootConfig `json:"roots"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode roots list: %v", err)
	}
	if len(listResp.Roots) != 1 {
		t.Fatalf("want 1 root, got %d", len(listResp.Roots))
	}

	// 2. POST /api/roots without CSRF header is rejected with 403
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/roots", strings.NewReader(`{"path":"/tmp"}`))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST without CSRF: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 for missing CSRF header, got %d", resp.StatusCode)
	}

	// 3. POST /api/roots with valid CSRF header adds a new root
	newRoot := t.TempDir()
	reqBody := `{"path":"` + newRoot + `"}`
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/roots", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-KB-UI", "1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST with CSRF: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("want 201 Created, got %d", resp.StatusCode)
	}

	// Verify root was added
	resp, err = http.Get(srv.URL + "/api/roots")
	if err != nil {
		t.Fatalf("GET /api/roots: %v", err)
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&listResp)
	if len(listResp.Roots) != 2 {
		t.Errorf("want 2 roots after add, got %d", len(listResp.Roots))
	}

	// 4. DELETE /api/roots removes the root
	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/api/roots?path="+url.QueryEscape(newRoot), nil)
	req.Header.Set("X-KB-UI", "1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE root: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 OK on delete, got %d", resp.StatusCode)
	}

	// Verify count is back to 1
	resp, err = http.Get(srv.URL + "/api/roots")
	if err != nil {
		t.Fatalf("GET /api/roots after delete: %v", err)
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&listResp)
	if len(listResp.Roots) != 1 {
		t.Errorf("want 1 root after remove, got %d", len(listResp.Roots))
	}
	_ = root
}

func TestServerControlEndpoints(t *testing.T) {
	srv, _, _, _ := setupTestServer(t)

	// 1. GET /api/control
	resp, err := http.Get(srv.URL + "/api/control")
	if err != nil {
		t.Fatalf("GET /api/control: %v", err)
	}
	defer resp.Body.Close()
	var ctrlState struct {
		Paused bool `json:"paused"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ctrlState)
	if ctrlState.Paused {
		t.Errorf("expected initially not paused")
	}

	// 2. POST /api/control/pause with X-KB-UI
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/control/pause", nil)
	req.Header.Set("X-KB-UI", "1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/control/pause: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 OK, got %d", resp.StatusCode)
	}

	// Verify paused
	resp, err = http.Get(srv.URL + "/api/control")
	if err != nil {
		t.Fatalf("GET /api/control paused: %v", err)
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&ctrlState)
	if !ctrlState.Paused {
		t.Errorf("expected paused after pause request")
	}

	// 3. POST /api/control/resume with X-KB-UI
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/control/resume", nil)
	req.Header.Set("X-KB-UI", "1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/control/resume: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 OK, got %d", resp.StatusCode)
	}

	// Verify resumed
	resp, err = http.Get(srv.URL + "/api/control")
	if err != nil {
		t.Fatalf("GET /api/control resumed: %v", err)
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&ctrlState)
	if ctrlState.Paused {
		t.Errorf("expected not paused after resume request")
	}
}

func TestServerForwardedPrefix(t *testing.T) {
	srv, _, _, _ := setupTestServer(t)

	cases := []struct {
		prefix   string
		wantBase string
	}{
		{"", `href="/"`},
		{"/kb", `href="/kb/"`},
		{"/app/kb", `href="/app/kb/"`},
		{`"><script>alert(1)</script>`, `href="/scriptalert1/script/"`},
	}

	for _, c := range cases {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
		if c.prefix != "" {
			req.Header.Set("X-Forwarded-Prefix", c.prefix)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET / with prefix %q: %v", c.prefix, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), c.wantBase) {
			t.Errorf("prefix %q: want base %q in HTML", c.prefix, c.wantBase)
		}
		// Confirm no unescaped script tag
		if strings.Contains(string(body), `<script>alert(1)</script>`) {
			t.Errorf("prefix %q allowed script injection in base href", c.prefix)
		}
	}
}

func TestNormalizeBindAddress(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", "127.0.0.1:6674"},
		{":6674", "127.0.0.1:6674"},
		{"6674", "127.0.0.1:6674"},
		{"0.0.0.0:6674", "127.0.0.1:6674"},
		{"127.0.0.1:6674", "127.0.0.1:6674"},
		{"localhost:7000", "localhost:7000"},
	}

	for _, c := range cases {
		got := normalizeBindAddress(c.input)
		if got != c.want {
			t.Errorf("normalizeBindAddress(%q): want %q, got %q", c.input, c.want, got)
		}
	}
}
