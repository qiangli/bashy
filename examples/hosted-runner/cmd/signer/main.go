// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

// Fake master signer and concurrent load generator for the H4/H5 hosted-runner
// pilot (test keys only). Signs X-Bashy-Hosted-Identity in the exact H1 wire
// format (`v1.<b64url>.<hmac>`) and executes Streamable HTTP MCP calls against
// `bashy mcp serve --hosted`.
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	hostedHeader       = "X-Bashy-Hosted-Identity"
	hostedTokenVersion = "v1"
	hostedSchema       = "bashy-hosted-identity-v1"
	defaultAccount     = "bench@example.test"
	defaultSecretsTok  = "bench-vault-token"
)

type hostedApproval struct {
	ApprovalID string   `json:"approval_id"`
	Digest     string   `json:"digest"`
	Effects    []string `json:"effects"`
}

type hostedIdentity struct {
	Schema         string          `json:"v"`
	Caller         string          `json:"caller"`
	TaskID         string          `json:"task_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	SecretsToken   string          `json:"secrets_token,omitempty"`
	Approval       *hostedApproval `json:"approval,omitempty"`
	IssuedAt       int64           `json:"iat"`
	ExpiresAt      int64           `json:"exp"`
}

type runToolInput struct {
	Name  string            `json:"name"`
	Args  []string          `json:"args,omitempty"`
	Stdin string            `json:"stdin,omitempty"`
	Dir   string            `json:"dir,omitempty"`
	Env   map[string]string `json:"env,omitempty"`
}

func callDigest(id hostedIdentity, tool string, arguments json.RawMessage) (string, bool) {
	var in runToolInput
	if len(arguments) > 0 {
		dec := json.NewDecoder(bytes.NewReader(arguments))
		dec.DisallowUnknownFields()
		if dec.Decode(&in) != nil {
			return "", false
		}
	}
	parts := []string{"bashy-hosted-call-v1", id.Caller, id.TaskID, tool, in.Name, in.Dir, in.Stdin, strconv.Itoa(len(in.Args))}
	parts = append(parts, in.Args...)
	parts = append(parts, strconv.Itoa(len(in.Env)))
	keys := make([]string, 0, len(in.Env))
	for k := range in.Env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		parts = append(parts, k+"="+in.Env[k])
	}
	sum := sha256.New()
	for _, part := range parts {
		fmt.Fprintf(sum, "%d:%s;", len(part), part)
	}
	return fmt.Sprintf("%x", sum.Sum(nil)), true
}

func signIdentity(key []byte, id hostedIdentity) (string, error) {
	raw, err := json.Marshal(id)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(hostedTokenVersion + "." + payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hostedTokenVersion + "." + payload + "." + sig, nil
}

type headerList []string

func (h *headerList) String() string { return strings.Join(*h, ", ") }
func (h *headerList) Set(v string) error {
	*h = append(*h, v)
	return nil
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type callResult struct {
	HTTPStatus int             `json:"http_status"`
	LatencyMs  float64         `json:"latency_ms"`
	E2EMs      float64         `json:"e2e_ms"`
	SessionID  string          `json:"session_id,omitempty"`
	Body       string          `json:"body,omitempty"`
	Structured json.RawMessage `json:"structured,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
	ExitCode   int             `json:"exit_code,omitempty"`
}

func parseMCPBody(ctype string, raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if strings.Contains(ctype, "text/event-stream") || bytes.HasPrefix(trimmed, []byte("event:")) || bytes.HasPrefix(trimmed, []byte("data:")) {
		for _, line := range strings.Split(string(trimmed), "\n") {
			line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
			if data, ok := strings.CutPrefix(line, "data:"); ok {
				data = strings.TrimSpace(data)
				if data != "" {
					return []byte(data), nil
				}
			}
		}
		return nil, fmt.Errorf("no data payload in SSE body: %s", trimmed)
	}
	return trimmed, nil
}

func postMCP(client *http.Client, endpoint, token string, extraHeaders []string, sessionID string, body any) (int, string, []byte, float64, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, "", nil, 0, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, "", nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set(hostedHeader, token)
	}
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	for _, kv := range extraHeaders {
		k, v, ok := strings.Cut(kv, ":")
		if !ok {
			return 0, "", nil, 0, fmt.Errorf("invalid --header %q (want Key: Value)", kv)
		}
		req.Header.Add(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	start := time.Now()
	resp, err := client.Do(req)
	elapsedMs := float64(time.Since(start).Nanoseconds()) / 1e6
	if err != nil {
		return 0, "", nil, elapsedMs, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", nil, elapsedMs, err
	}
	return resp.StatusCode, resp.Header.Get("Mcp-Session-Id"), raw, elapsedMs, nil
}

func doSignedCall(client *http.Client, endpoint, token string, extraHeaders []string, tool string, args json.RawMessage) (callResult, error) {
	e2eStart := time.Now()
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "hosted-runner-signer", "version": "1"},
		},
	}
	status, sid, raw, _, err := postMCP(client, endpoint, token, extraHeaders, "", initReq)
	e2eMs := func() float64 { return float64(time.Since(e2eStart).Nanoseconds()) / 1e6 }
	if err != nil {
		return callResult{E2EMs: e2eMs()}, err
	}
	if status != http.StatusOK {
		return callResult{HTTPStatus: status, E2EMs: e2eMs(), Body: strings.TrimSpace(string(raw))}, fmt.Errorf("initialize returned HTTP %d: %s", status, strings.TrimSpace(string(raw)))
	}

	notifReq := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	nStatus, _, nRaw, _, err := postMCP(client, endpoint, token, extraHeaders, sid, notifReq)
	if err != nil {
		return callResult{E2EMs: e2eMs()}, err
	}
	if nStatus < 200 || nStatus >= 300 {
		return callResult{HTTPStatus: nStatus, E2EMs: e2eMs(), Body: strings.TrimSpace(string(nRaw))}, fmt.Errorf("notifications/initialized returned HTTP %d: %s", nStatus, strings.TrimSpace(string(nRaw)))
	}

	callReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      tool,
			"arguments": args,
		},
	}
	cStatus, _, cRaw, latMs, err := postMCP(client, endpoint, token, extraHeaders, sid, callReq)
	totalMs := e2eMs()
	if err != nil {
		return callResult{HTTPStatus: cStatus, LatencyMs: latMs, E2EMs: totalMs}, err
	}
	res := callResult{HTTPStatus: cStatus, LatencyMs: latMs, E2EMs: totalMs, SessionID: sid, Body: strings.TrimSpace(string(cRaw))}
	if cStatus != http.StatusOK {
		return res, fmt.Errorf("tools/call returned HTTP %d: %s", cStatus, res.Body)
	}
	jsonBytes, err := parseMCPBody("", cRaw)
	if err != nil {
		return res, err
	}
	var rpc rpcResponse
	if err := json.Unmarshal(jsonBytes, &rpc); err != nil {
		return res, fmt.Errorf("decode rpc: %w (%s)", err, jsonBytes)
	}
	if rpc.Error != nil {
		return res, fmt.Errorf("rpc error %d: %s", rpc.Error.Code, rpc.Error.Message)
	}
	var toolRes struct {
		IsError           bool            `json:"isError"`
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if err := json.Unmarshal(rpc.Result, &toolRes); err == nil {
		res.IsError = toolRes.IsError
		res.Structured = toolRes.StructuredContent
		var exitProbe struct {
			ExitCode int `json:"exit_code"`
		}
		if json.Unmarshal(toolRes.StructuredContent, &exitProbe) == nil {
			res.ExitCode = exitProbe.ExitCode
		}
	}
	return res, nil
}

func doToolsList(client *http.Client, endpoint, token string, extraHeaders []string) ([]string, error) {
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "hosted-runner-signer", "version": "1"},
		},
	}
	status, sid, raw, _, err := postMCP(client, endpoint, token, extraHeaders, "", initReq)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("initialize HTTP %d: %s", status, strings.TrimSpace(string(raw)))
	}
	_, _, _, _, _ = postMCP(client, endpoint, token, extraHeaders, sid, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})

	var names []string
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		lStatus, _, lRaw, _, err := postMCP(client, endpoint, token, extraHeaders, sid, map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "tools/list",
			"params":  params,
		})
		if err != nil {
			return nil, err
		}
		if lStatus != http.StatusOK {
			return nil, fmt.Errorf("tools/list HTTP %d: %s", lStatus, strings.TrimSpace(string(lRaw)))
		}
		jb, err := parseMCPBody("", lRaw)
		if err != nil {
			return nil, err
		}
		var rpc rpcResponse
		if err := json.Unmarshal(jb, &rpc); err != nil {
			return nil, err
		}
		if rpc.Error != nil {
			return nil, fmt.Errorf("tools/list rpc error %d: %s", rpc.Error.Code, rpc.Error.Message)
		}
		var page struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(rpc.Result, &page); err != nil {
			return nil, err
		}
		for _, t := range page.Tools {
			names = append(names, t.Name)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return names, nil
}

func startFakeVault(listenAddr string) (string, func(), error) {
	items := map[string]string{
		"trove/layout": `{"schema_version":"bashy-trove-v1","/drive":{"locator":"drive:My Drive/Project","account":"` + defaultAccount + `"}}`,
		"google/" + defaultAccount + "/credentials": `{"type":"authorized_user","client_id":"fake-client","client_secret":"fake-secret","refresh_token":"fake-refresh"}`,
		"github/work/token":                         "fake-github-token",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/secrets", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if r.Method != http.MethodGet || !strings.HasPrefix(auth, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) == "" {
			http.Error(w, "unauthorized", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		names := make([]map[string]string, 0, len(items))
		for k := range items {
			names = append(names, map[string]string{"name": k})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"secrets": names})
	})
	mux.HandleFunc("/api/v1/secrets/", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if r.Method != http.MethodGet || !strings.HasPrefix(auth, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) == "" {
			http.Error(w, "unauthorized", http.StatusForbidden)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/api/v1/secrets/")
		val, ok := items[name]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"name": name, "value": val})
	})

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return "", nil, err
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return "http://" + ln.Addr().String(), func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}, nil
}

type workloadSpec struct {
	Label           string
	Tool            string
	Args            json.RawMessage
	NeedsSecrets    bool
	ApprovalEffects []string
}

func resolveWorkloadMix(csv string) ([]workloadSpec, error) {
	parts := strings.Split(csv, ",")
	out := make([]workloadSpec, 0, len(parts))
	for _, raw := range parts {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		switch name {
		case "echo":
			out = append(out, workloadSpec{
				Label: "echo",
				Tool:  "run_tool",
				Args:  json.RawMessage(`{"name":"echo","args":["hello","hosted"]}`),
			})
		case "gmail", "gmail-list":
			out = append(out, workloadSpec{
				Label:        "gmail",
				Tool:         "google",
				Args:         json.RawMessage(`{"args":["gmail-list","account=` + defaultAccount + `","maxResults:=5"]}`),
				NeedsSecrets: true,
			})
		case "trove":
			out = append(out, workloadSpec{
				Label:        "trove",
				Tool:         "trove",
				Args:         json.RawMessage(`{"args":["ls","/drive","--json"]}`),
				NeedsSecrets: true,
			})
		case "gh", "gh-pr-list":
			out = append(out, workloadSpec{
				Label:           "gh",
				Tool:            "run_tool",
				Args:            json.RawMessage(`{"name":"gh","args":["pr","list"]}`),
				ApprovalEffects: []string{"cred"},
			})
		default:
			return nil, fmt.Errorf("unknown --mix entry %q (allowed: echo, gmail, trove, gh)", name)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("--mix must contain at least one tool")
	}
	return out, nil
}

type latencyStats struct {
	Count  int     `json:"count"`
	MinMs  float64 `json:"min_ms"`
	MeanMs float64 `json:"mean_ms"`
	P50Ms  float64 `json:"p50_ms"`
	P95Ms  float64 `json:"p95_ms"`
	P99Ms  float64 `json:"p99_ms"`
	MaxMs  float64 `json:"max_ms"`
}

func computeLatencyStats(samples []float64) latencyStats {
	if len(samples) == 0 {
		return latencyStats{}
	}
	sorted := slices.Clone(samples)
	sort.Float64s(sorted)
	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	pct := func(p float64) float64 {
		idx := int(math.Ceil(p/100.0*float64(len(sorted)))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return math.Round(sorted[idx]*100) / 100
	}
	round2 := func(v float64) float64 { return math.Round(v*100) / 100 }
	return latencyStats{
		Count:  len(sorted),
		MinMs:  round2(sorted[0]),
		MeanMs: round2(sum / float64(len(sorted))),
		P50Ms:  pct(50),
		P95Ms:  pct(95),
		P99Ms:  pct(99),
		MaxMs:  round2(sorted[len(sorted)-1]),
	}
}

type loadOutcome struct {
	seq        int64
	tool       string
	httpStatus int
	e2eMs      float64
	callMs     float64
	timeout    bool
	errMsg     string
}

type toolLoadSummary struct {
	Total       int          `json:"total"`
	OK          int          `json:"ok"`
	Errors      int          `json:"errors"`
	Timeouts    int          `json:"timeouts"`
	FirstE2EMs  float64      `json:"first_e2e_ms"`
	E2E         latencyStats `json:"e2e"`
	ToolsCall   latencyStats `json:"tools_call"`
}

type loadSummary struct {
	Mode             string                     `json:"mode"`
	URL              string                     `json:"url"`
	Mix              []string                   `json:"mix"`
	Concurrency      int                        `json:"concurrency"`
	TargetRateRPS    int                        `json:"target_rate_rps,omitempty"`
	TargetDurationMs float64                    `json:"target_duration_ms,omitempty"`
	TargetBurst      int                        `json:"target_burst,omitempty"`
	Total            int                        `json:"total"`
	OK               int                        `json:"ok"`
	Errors           int                        `json:"errors"`
	Timeouts         int                        `json:"timeouts"`
	ErrorRate        float64                    `json:"error_rate"`
	ElapsedMs        float64                    `json:"elapsed_ms"`
	AchievedRPS      float64                    `json:"achieved_rps"`
	ColdFirstE2EMs   float64                    `json:"cold_first_e2e_ms"`
	E2E              latencyStats               `json:"e2e"`
	ToolsCall        latencyStats               `json:"tools_call"`
	HTTPStatuses     map[string]int             `json:"http_statuses"`
	ByTool           map[string]toolLoadSummary `json:"by_tool"`
	SampleErrors     []string                   `json:"sample_errors,omitempty"`
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}

func runLoad(
	client *http.Client,
	key []byte,
	endpoint string,
	extraHeaders []string,
	baseCaller string,
	baseTaskID string,
	secretsTok string,
	ttl time.Duration,
	specs []workloadSpec,
	rate int,
	duration time.Duration,
	burst int,
	concurrency int,
	openLoop bool,
	queueCap int,
) loadSummary {
	if concurrency < 1 {
		concurrency = 1
	}
	if queueCap < 1 {
		queueCap = concurrency * 2
	}
	jobs := make(chan int64, queueCap)
	results := make(chan loadOutcome, 1024)

	var wg sync.WaitGroup
	baseNano := time.Now().UnixNano()
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seq := range jobs {
				spec := specs[int(seq)%len(specs)]
				now := time.Now()
				id := hostedIdentity{
					Schema:         hostedSchema,
					Caller:         fmt.Sprintf("%s-%d", baseCaller, seq),
					TaskID:         fmt.Sprintf("%s-%d", baseTaskID, seq),
					IdempotencyKey: fmt.Sprintf("idem-h5-%d-%d", baseNano, seq),
					IssuedAt:       now.Unix(),
					ExpiresAt:      now.Add(ttl).Unix(),
				}
				if spec.NeedsSecrets {
					id.SecretsToken = secretsTok
				}
				if len(spec.ApprovalEffects) > 0 {
					if dig, ok := callDigest(id, spec.Tool, spec.Args); ok {
						id.Approval = &hostedApproval{
							ApprovalID: fmt.Sprintf("ap-h5-%d", seq),
							Digest:     dig,
							Effects:    spec.ApprovalEffects,
						}
					}
				}
				tok, err := signIdentity(key, id)
				if err != nil {
					results <- loadOutcome{seq: seq, tool: spec.Label, errMsg: err.Error()}
					continue
				}
				res, err := doSignedCall(client, endpoint, tok, extraHeaders, spec.Tool, spec.Args)
				out := loadOutcome{
					seq:        seq,
					tool:       spec.Label,
					httpStatus: res.HTTPStatus,
					e2eMs:      res.E2EMs,
					callMs:     res.LatencyMs,
				}
				if err != nil {
					out.timeout = isTimeoutError(err)
					out.errMsg = err.Error()
				} else if res.IsError || res.ExitCode != 0 {
					out.errMsg = fmt.Sprintf("tool error is_error=%t exit_code=%d body=%s", res.IsError, res.ExitCode, res.Body)
				}
				results <- out
			}
		}()
	}

	var collected []loadOutcome
	collectDone := make(chan struct{})
	go func() {
		defer close(collectDone)
		for o := range results {
			collected = append(collected, o)
		}
	}()

	start := time.Now()
	if rate > 0 {
		totalPlanned := int64(math.Round(float64(rate) * duration.Seconds()))
		if totalPlanned < 1 {
			totalPlanned = 1
		}
		interval := time.Second / time.Duration(rate)
		for i := int64(0); i < totalPlanned; i++ {
			targetAt := start.Add(time.Duration(i) * interval)
			if wait := time.Until(targetAt); wait > 0 {
				time.Sleep(wait)
			}
			if openLoop {
				select {
				case jobs <- i:
				default:
					spec := specs[int(i)%len(specs)]
					results <- loadOutcome{seq: i, tool: spec.Label, httpStatus: 503, errMsg: "queue saturated (open-loop admission drop)"}
				}
			} else {
				jobs <- i
			}
		}
	} else {
		n := int64(burst)
		if n < 1 {
			n = 1
		}
		for i := int64(0); i < n; i++ {
			jobs <- i
		}
	}
	close(jobs)
	wg.Wait()
	elapsed := time.Since(start)
	close(results)
	<-collectDone

	sort.Slice(collected, func(i, j int) bool { return collected[i].seq < collected[j].seq })

	mixNames := make([]string, len(specs))
	for i, s := range specs {
		mixNames[i] = s.Label
	}
	sum := loadSummary{
		Mode:         "burst",
		URL:          endpoint,
		Mix:          mixNames,
		Concurrency:  concurrency,
		Total:        len(collected),
		ElapsedMs:    math.Round(float64(elapsed.Nanoseconds())/1e6*100) / 100,
		HTTPStatuses: map[string]int{},
		ByTool:       map[string]toolLoadSummary{},
	}
	if rate > 0 {
		sum.Mode = "sustained"
		sum.TargetRateRPS = rate
		sum.TargetDurationMs = float64(duration.Milliseconds())
	} else {
		sum.TargetBurst = burst
	}
	if elapsed > 0 {
		sum.AchievedRPS = math.Round(float64(len(collected))/elapsed.Seconds()*100) / 100
	}

	var allE2E, allCall []float64
	perToolE2E := map[string][]float64{}
	perToolCall := map[string][]float64{}
	perToolMeta := map[string]*toolLoadSummary{}

	for _, o := range collected {
		stKey := strconv.Itoa(o.httpStatus)
		sum.HTTPStatuses[stKey]++
		tm := perToolMeta[o.tool]
		if tm == nil {
			tm = &toolLoadSummary{FirstE2EMs: math.Round(o.e2eMs*100) / 100}
			perToolMeta[o.tool] = tm
		}
		tm.Total++
		if o.seq == 0 {
			sum.ColdFirstE2EMs = math.Round(o.e2eMs*100) / 100
		}
		if o.errMsg == "" {
			sum.OK++
			tm.OK++
			allE2E = append(allE2E, o.e2eMs)
			allCall = append(allCall, o.callMs)
			perToolE2E[o.tool] = append(perToolE2E[o.tool], o.e2eMs)
			perToolCall[o.tool] = append(perToolCall[o.tool], o.callMs)
		} else {
			sum.Errors++
			tm.Errors++
			if o.timeout {
				sum.Timeouts++
				tm.Timeouts++
			}
			if len(sum.SampleErrors) < 5 {
				sum.SampleErrors = append(sum.SampleErrors, fmt.Sprintf("seq=%d tool=%s: %s", o.seq, o.tool, o.errMsg))
			}
		}
	}

	if sum.Total > 0 {
		sum.ErrorRate = math.Round(float64(sum.Errors)/float64(sum.Total)*10000) / 10000
	}
	sum.E2E = computeLatencyStats(allE2E)
	sum.ToolsCall = computeLatencyStats(allCall)
	for name, tm := range perToolMeta {
		tm.E2E = computeLatencyStats(perToolE2E[name])
		tm.ToolsCall = computeLatencyStats(perToolCall[name])
		sum.ByTool[name] = *tm
	}
	return sum
}

func main() {
	var (
		endpoint        = flag.String("url", "http://127.0.0.1:22751/mcp", "hosted runner /mcp URL")
		keyFile         = flag.String("key-file", "", "path to 0600 runner HMAC key file (test key only)")
		caller          = flag.String("caller", "pilot-caller@example.test", "caller identity")
		taskID          = flag.String("task-id", fmt.Sprintf("job-pilot-%d", time.Now().UnixNano()), "task id")
		idempotencyKey  = flag.String("idempotency-key", "", "idempotency key (default: unique per run)")
		secretsToken    = flag.String("secrets-token", "", "optional secrets_token")
		approveEffects  = flag.String("approve-effects", "", "comma-separated gated effects to approve via digest (e.g. cred)")
		ttl             = flag.Duration("ttl", time.Minute, "identity TTL (<= 5m)")
		tool            = flag.String("tool", "run_tool", "MCP tool name")
		argsJSON        = flag.String("args", `{"name":"echo","args":["hello","hosted"]}`, "JSON arguments for tools/call")
		unsigned        = flag.Bool("unsigned", false, "send unsigned tools/list probe (expects HTTP 401)")
		verify          = flag.Bool("verify", false, "run Step 2 verification suite: unsigned 401, tools/list check, run_tool echo, replay")
		loadMode        = flag.Bool("load", false, "run concurrent load test against --url")
		mixCSV          = flag.String("mix", "echo,gmail,trove,gh", "comma-separated workload mix for --load (echo,gmail,trove,gh)")
		rate            = flag.Int("rate", 0, "target sustained calls/sec for --load (0 = burst mode)")
		duration        = flag.Duration("duration", 60*time.Second, "sustained load duration when --rate > 0")
		burst           = flag.Int("burst", 1000, "total calls for burst mode when --rate == 0")
		concurrency     = flag.Int("concurrency", 64, "concurrent worker count for --load")
		openLoop        = flag.Bool("open-loop", false, "drop excess arrivals as 503 queue_saturated instead of throttling the pacer")
		queueCap        = flag.Int("queue-cap", 0, "buffered job queue capacity for --load (default: 2*concurrency)")
		callTimeout     = flag.Duration("call-timeout", 30*time.Second, "per-request HTTP client timeout")
		fakeVaultListen = flag.String("fake-vault-listen", "", "optional 127.0.0.1:PORT for in-process fake vault during --load/call")
		serveVault      = flag.String("serve-vault", "", "run only the fake vault server on ADDR until SIGINT/SIGTERM")
		asJSON          = flag.Bool("json", false, "emit JSON output")
		extraHeaders    headerList
	)
	flag.Var(&extraHeaders, "header", "extra HTTP header 'Key: Value' (repeatable)")
	flag.Parse()

	if *serveVault != "" {
		url, stop, err := startFakeVault(*serveVault)
		if err != nil {
			fmt.Fprintf(os.Stderr, "signer: serve-vault: %v\n", err)
			os.Exit(1)
		}
		defer stop()
		if *asJSON {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"fake_vault_url": url})
		} else {
			fmt.Printf("fake vault listening on %s\n", url)
		}
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		return
	}

	var stopVault func()
	if *fakeVaultListen != "" {
		var err error
		_, stopVault, err = startFakeVault(*fakeVaultListen)
		if err != nil {
			fmt.Fprintf(os.Stderr, "signer: fake-vault: %v\n", err)
			os.Exit(1)
		}
		defer stopVault()
	}

	transport := &http.Transport{
		MaxIdleConns:        512,
		MaxIdleConnsPerHost: 512,
		IdleConnTimeout:     90 * time.Second,
	}
	client := &http.Client{Timeout: *callTimeout, Transport: transport}

	if *unsigned {
		status, _, raw, latMs, err := postMCP(client, *endpoint, "", extraHeaders, "", map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/list",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "signer: request error: %v\n", err)
			os.Exit(1)
		}
		out := map[string]any{"unsigned_status": status, "latency_ms": latMs, "body": strings.TrimSpace(string(raw))}
		if *asJSON {
			_ = json.NewEncoder(os.Stdout).Encode(out)
		} else {
			fmt.Printf("unsigned status=%d latency_ms=%.2f\n", status, latMs)
		}
		if status != http.StatusUnauthorized {
			os.Exit(1)
		}
		return
	}

	if *keyFile == "" {
		fmt.Fprintln(os.Stderr, "signer: --key-file is required")
		os.Exit(2)
	}
	rawKey, err := os.ReadFile(*keyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "signer: read key: %v\n", err)
		os.Exit(1)
	}
	key := bytes.TrimSpace(rawKey)
	if len(key) < 16 {
		fmt.Fprintf(os.Stderr, "signer: key too short (%d bytes, need >= 16)\n", len(key))
		os.Exit(1)
	}

	if *loadMode {
		specs, err := resolveWorkloadMix(*mixCSV)
		if err != nil {
			fmt.Fprintf(os.Stderr, "signer: %v\n", err)
			os.Exit(2)
		}
		tok := *secretsToken
		if tok == "" {
			tok = defaultSecretsTok
		}
		summary := runLoad(client, key, *endpoint, extraHeaders, *caller, *taskID, tok, *ttl, specs, *rate, *duration, *burst, *concurrency, *openLoop, *queueCap)
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(summary)
		} else {
			fmt.Printf("mode=%s total=%d ok=%d errors=%d timeouts=%d elapsed_ms=%.2f rps=%.2f p50=%.2fms p95=%.2fms p99=%.2fms\n",
				summary.Mode, summary.Total, summary.OK, summary.Errors, summary.Timeouts, summary.ElapsedMs, summary.AchievedRPS,
				summary.E2E.P50Ms, summary.E2E.P95Ms, summary.E2E.P99Ms)
		}
		if summary.Errors > 0 {
			os.Exit(1)
		}
		return
	}

	ikey := *idempotencyKey
	if ikey == "" {
		ikey = fmt.Sprintf("idem-%d", time.Now().UnixNano())
	}
	now := time.Now()
	id := hostedIdentity{
		Schema:         hostedSchema,
		Caller:         *caller,
		TaskID:         *taskID,
		IdempotencyKey: ikey,
		SecretsToken:   *secretsToken,
		IssuedAt:       now.Unix(),
		ExpiresAt:      now.Add(*ttl).Unix(),
	}
	if strings.TrimSpace(*approveEffects) != "" {
		var effs []string
		for _, e := range strings.Split(*approveEffects, ",") {
			if e = strings.TrimSpace(e); e != "" {
				effs = append(effs, e)
			}
		}
		if len(effs) > 0 {
			dig, ok := callDigest(id, *tool, json.RawMessage(*argsJSON))
			if !ok {
				fmt.Fprintln(os.Stderr, "signer: cannot compute call digest for --approve-effects")
				os.Exit(2)
			}
			id.Approval = &hostedApproval{ApprovalID: "ap-" + ikey, Digest: dig, Effects: effs}
		}
	}
	token, err := signIdentity(key, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "signer: sign: %v\n", err)
		os.Exit(1)
	}

	if !*verify {
		res, err := doSignedCall(client, *endpoint, token, extraHeaders, *tool, json.RawMessage(*argsJSON))
		if *asJSON {
			_ = json.NewEncoder(os.Stdout).Encode(res)
		} else {
			fmt.Printf("status=%d latency_ms=%.2f e2e_ms=%.2f structured=%s\n", res.HTTPStatus, res.LatencyMs, res.E2EMs, string(res.Structured))
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "signer: %v\n", err)
			os.Exit(1)
		}
		return
	}

	uStatus, _, uRaw, uLatMs, err := postMCP(client, *endpoint, "", extraHeaders, "", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "signer: unsigned probe error: %v\n", err)
		os.Exit(1)
	}
	if uStatus != http.StatusUnauthorized {
		fmt.Fprintf(os.Stderr, "signer: unsigned probe got HTTP %d (%s), want 401\n", uStatus, strings.TrimSpace(string(uRaw)))
		os.Exit(1)
	}

	tools, err := doToolsList(client, *endpoint, token, extraHeaders)
	if err != nil {
		fmt.Fprintf(os.Stderr, "signer: tools/list error: %v\n", err)
		os.Exit(1)
	}
	toolSet := map[string]bool{}
	for _, name := range tools {
		toolSet[name] = true
	}
	for _, forbidden := range []string{"shell_open", "shell_exec", "shell_close"} {
		if toolSet[forbidden] {
			fmt.Fprintf(os.Stderr, "signer: tools/list unexpectedly includes %s\n", forbidden)
			os.Exit(1)
		}
	}
	if !toolSet["run_tool"] {
		fmt.Fprintln(os.Stderr, "signer: tools/list missing run_tool")
		os.Exit(1)
	}

	firstRes, err := doSignedCall(client, *endpoint, token, extraHeaders, *tool, json.RawMessage(*argsJSON))
	if err != nil {
		fmt.Fprintf(os.Stderr, "signer: signed call error: %v\n", err)
		os.Exit(1)
	}
	var echoOut struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal(firstRes.Structured, &echoOut); err != nil {
		fmt.Fprintf(os.Stderr, "signer: decode first result %s: %v\n", firstRes.Structured, err)
		os.Exit(1)
	}
	if echoOut.ExitCode != 0 {
		fmt.Fprintf(os.Stderr, "signer: signed call failed exit=%d stderr=%q\n", echoOut.ExitCode, echoOut.Stderr)
		os.Exit(1)
	}

	replayRes, err := doSignedCall(client, *endpoint, token, extraHeaders, *tool, json.RawMessage(*argsJSON))
	if err != nil {
		fmt.Fprintf(os.Stderr, "signer: replay call error: %v\n", err)
		os.Exit(1)
	}
	var replayOut struct {
		State  string `json:"state"`
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(replayRes.Structured, &replayOut); err != nil {
		fmt.Fprintf(os.Stderr, "signer: decode replay result %s: %v\n", replayRes.Structured, err)
		os.Exit(1)
	}
	if replayOut.State != "replayed" {
		fmt.Fprintf(os.Stderr, "signer: replay state = %q, want replayed (structured=%s)\n", replayOut.State, replayRes.Structured)
		os.Exit(1)
	}

	summary := map[string]any{
		"ok":                  true,
		"url":                 *endpoint,
		"unsigned_status":     uStatus,
		"unsigned_latency_ms": uLatMs,
		"tools_count":         len(tools),
		"has_google":          toolSet["google"],
		"has_trove":           toolSet["trove"],
		"call_status":         firstRes.HTTPStatus,
		"call_latency_ms":     firstRes.LatencyMs,
		"call_stdout":         echoOut.Stdout,
		"call_exit_code":      echoOut.ExitCode,
		"replay_status":       replayRes.HTTPStatus,
		"replay_latency_ms":   replayRes.LatencyMs,
		"replay_state":        replayOut.State,
		"idempotency_key":     ikey,
		"task_id":             *taskID,
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(summary)
	} else {
		fmt.Printf("ok=true unsigned=%d call_latency_ms=%.2f replay_latency_ms=%.2f stdout=%q replay=%s tools=%d google=%t trove=%t\n",
			uStatus, firstRes.LatencyMs, replayRes.LatencyMs, echoOut.Stdout, replayOut.State, len(tools), toolSet["google"], toolSet["trove"])
	}
}
