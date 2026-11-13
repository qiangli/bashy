package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Owner tracing is off unless the command wrote an endpoint for this run.
// Only method and status are exported; record content and identities stay local.
func ownerTrace(handler http.Handler, data string) http.Handler {
	home := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(data))))
	b, err := os.ReadFile(filepath.Join(home, "table", "otel-endpoint"))
	if err != nil { return handler }
	u, err := url.Parse(strings.TrimSpace(string(b)))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" { return handler }
	if !strings.HasSuffix(u.Path, "/v1/traces") { u.Path = strings.TrimRight(u.Path, "/") + "/v1/traces" }
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") { handler.ServeHTTP(w, r); return }
		traceID, parentID := readTraceparent(r.Header.Get("traceparent"))
		if traceID == "" { traceID = randomHex(16) }
		spanID := randomHex(8)
		r.Header.Set("traceparent", "00-"+traceID+"-"+spanID+"-01")
		start := time.Now()
		status := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		handler.ServeHTTP(status, r)
		end := time.Now()
		go exportRequestSpan(u.String(), traceID, parentID, spanID, r.Method, status.code, start, end)
	})
}

type statusWriter struct { http.ResponseWriter; code int }
func (w *statusWriter) WriteHeader(code int) { w.code = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Flush() { if f, ok := w.ResponseWriter.(http.Flusher); ok { f.Flush() } }

func randomHex(n int) string { b := make([]byte, n); _, _ = rand.Read(b); return hex.EncodeToString(b) }

func readTraceparent(v string) (string, string) {
	p := strings.Split(v, "-")
	if len(p) != 4 || p[0] != "00" || len(p[1]) != 32 || len(p[2]) != 16 || len(p[3]) != 2 { return "", "" }
	for _, s := range p[1:] { if _, err := hex.DecodeString(s); err != nil { return "", "" } }
	if p[1] == strings.Repeat("0", 32) || p[2] == strings.Repeat("0", 16) { return "", "" }
	return p[1], p[2]
}

func exportRequestSpan(endpoint, traceID, parentID, spanID, method string, status int, start, end time.Time) {
	span := map[string]any{
		"traceId": traceID, "spanId": spanID, "name": "table.request", "kind": 2,
		"startTimeUnixNano": start.UnixNano(), "endTimeUnixNano": end.UnixNano(),
		"attributes": []any{
			map[string]any{"key":"http.request.method", "value":map[string]any{"stringValue":method}},
			map[string]any{"key":"http.response.status_code", "value":map[string]any{"intValue":status}},
		},
	}
	if parentID != "" { span["parentSpanId"] = parentID }
	body, _ := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{
		"resource":map[string]any{"attributes":[]any{map[string]any{"key":"service.name","value":map[string]any{"stringValue":"table"}}}},
		"scopeSpans":[]any{map[string]any{"scope":map[string]any{"name":"bashy.examples.table"},"spans":[]any{span}}},
	}}})
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil { return }
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 2*time.Second}
	resp, err := client.Do(req)
	if err == nil { _ = resp.Body.Close() }
}
