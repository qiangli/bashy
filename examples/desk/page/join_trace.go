package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var participantTraceClient = http.Client{
	Timeout: 2 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}
var participantTraceSlots = make(chan struct{}, 16)

// traceParticipant follows the desk apps' owner-configured OTLP HTTP export.
// Read the endpoint from the server configuration, never from request input.
func traceParticipant(r *http.Request, p Participant, start time.Time) func() {
	raw := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"))
	generic := raw == ""
	if generic {
		raw = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	}
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return func() {}
	}

	if generic && !strings.HasSuffix(endpoint.Path, "/v1/traces") {
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/traces"
	}

	traceID := traceHex(16)
	parent := ""
	pieces := strings.Split(r.Header.Get("traceparent"), "-")
	if len(pieces) == 4 && pieces[0] == "00" && validTraceHex(pieces[1], 32) && validTraceHex(pieces[2], 16) {
		traceID, parent = pieces[1], pieces[2]
	}
	spanID := traceHex(8)
	r.Header.Set("traceparent", "00-"+traceID+"-"+spanID+"-01")
	return func() {
		end := time.Now()
		select {
		case participantTraceSlots <- struct{}{}:
		default:
			return
		}
		go func() {
			defer func() { <-participantTraceSlots }()
			span := map[string]any{
				"name": "page.request", "kind": 2, "traceId": traceID, "spanId": spanID,
				"startTimeUnixNano": strconv.FormatInt(start.UnixNano(), 10),
				"endTimeUnixNano":   strconv.FormatInt(end.UnixNano(), 10),
				"attributes": []any{
					traceAttr("http.request.method", r.Method),
					traceAttr("participant.handle", p.Handle),
					traceAttr("participant.auth", p.Auth),
					traceAttr("participant.address", p.Address),
				},
			}
			if parent != "" {
				span["parentSpanId"] = parent
			}
			body, _ := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{
				"resource": map[string]any{"attributes": []any{traceAttr("service.name", "page")}},
				"scopeSpans": []any{map[string]any{
					"scope": map[string]any{"name": "bashy.examples.desk.page"},
					"spans": []any{span},
				}},
			}}})
			resp, err := participantTraceClient.Post(endpoint.String(), "application/json", bytes.NewReader(body))
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
	}
}

func traceAttr(key, value string) map[string]any {
	return map[string]any{"key": key, "value": map[string]string{"stringValue": value}}
}

func traceHex(n int) string {
	buf := make([]byte, n)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

func validTraceHex(value string, length int) bool {
	if len(value) != length || strings.Trim(value, "0") == "" {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
