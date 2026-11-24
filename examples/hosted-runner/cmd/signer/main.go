// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

// Fake master signer for the H4 hosted-runner pilot (test keys only).
// Signs X-Bashy-Hosted-Identity in the exact H1 wire format (`v1.<b64url>.<hmac>`)
// and executes Streamable HTTP MCP calls against `bashy mcp serve --hosted`.
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	hostedHeader       = "X-Bashy-Hosted-Identity"
	hostedTokenVersion = "v1"
	hostedSchema       = "bashy-hosted-identity-v1"
)

type hostedIdentity struct {
	Schema         string `json:"v"`
	Caller         string `json:"caller"`
	TaskID         string `json:"task_id"`
	IdempotencyKey string `json:"idempotency_key"`
	SecretsToken   string `json:"secrets_token,omitempty"`
	IssuedAt       int64  `json:"iat"`
	ExpiresAt      int64  `json:"exp"`
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
	SessionID  string          `json:"session_id,omitempty"`
	Body       string          `json:"body,omitempty"`
	Structured json.RawMessage `json:"structured,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
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
		return callResult{}, err
	}
	if status != http.StatusOK {
		return callResult{HTTPStatus: status, Body: strings.TrimSpace(string(raw))}, fmt.Errorf("initialize returned HTTP %d: %s", status, strings.TrimSpace(string(raw)))
	}

	notifReq := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	nStatus, _, nRaw, _, err := postMCP(client, endpoint, token, extraHeaders, sid, notifReq)
	if err != nil {
		return callResult{}, err
	}
	if nStatus < 200 || nStatus >= 300 {
		return callResult{HTTPStatus: nStatus, Body: strings.TrimSpace(string(nRaw))}, fmt.Errorf("notifications/initialized returned HTTP %d: %s", nStatus, strings.TrimSpace(string(nRaw)))
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
	if err != nil {
		return callResult{}, err
	}
	res := callResult{HTTPStatus: cStatus, LatencyMs: latMs, SessionID: sid, Body: strings.TrimSpace(string(cRaw))}
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

func main() {
	var (
		endpoint       = flag.String("url", "http://127.0.0.1:22751/mcp", "hosted runner /mcp URL")
		keyFile        = flag.String("key-file", "", "path to 0600 runner HMAC key file (test key only)")
		caller         = flag.String("caller", "pilot-caller@example.test", "caller identity")
		taskID         = flag.String("task-id", fmt.Sprintf("job-pilot-%d", time.Now().UnixNano()), "task id")
		idempotencyKey = flag.String("idempotency-key", "", "idempotency key (default: unique per run)")
		secretsToken   = flag.String("secrets-token", "", "optional secrets_token")
		ttl            = flag.Duration("ttl", time.Minute, "identity TTL (<= 5m)")
		tool           = flag.String("tool", "run_tool", "MCP tool name")
		argsJSON       = flag.String("args", `{"name":"echo","args":["hello","hosted"]}`, "JSON arguments for tools/call")
		unsigned       = flag.Bool("unsigned", false, "send unsigned tools/list probe (expects HTTP 401)")
		verify         = flag.Bool("verify", false, "run Step 2 verification suite: unsigned 401, tools/list check, run_tool echo, replay")
		asJSON         = flag.Bool("json", false, "emit JSON output")
		extraHeaders   headerList
	)
	flag.Var(&extraHeaders, "header", "extra HTTP header 'Key: Value' (repeatable)")
	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}

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
			fmt.Printf("status=%d latency_ms=%.2f structured=%s\n", res.HTTPStatus, res.LatencyMs, string(res.Structured))
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "signer: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Verification suite:
	// 1. Unsigned request -> 401
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

	// 2. tools/list -> session tools absent; run_tool present
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

	// 3. Signed call
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

	// 4. Replay with same identity -> state: "replayed"
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
		"ok":                   true,
		"url":                  *endpoint,
		"unsigned_status":      uStatus,
		"unsigned_latency_ms":  uLatMs,
		"tools_count":          len(tools),
		"has_google":           toolSet["google"],
		"has_trove":            toolSet["trove"],
		"call_status":          firstRes.HTTPStatus,
		"call_latency_ms":      firstRes.LatencyMs,
		"call_stdout":          echoOut.Stdout,
		"call_exit_code":       echoOut.ExitCode,
		"replay_status":        replayRes.HTTPStatus,
		"replay_latency_ms":    replayRes.LatencyMs,
		"replay_state":         replayOut.State,
		"idempotency_key":      ikey,
		"task_id":              *taskID,
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
