package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func executeShapeRemote(ctx context.Context, r shapeCLIRequest) (any, error) {
	u, err := shapeConsoleURL(r.Remote)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, fmt.Errorf("shape: remote session cookies require HTTPS except on loopback")
		}
	}
	if r.CookieFile == "" {
		return nil, fmt.Errorf("shape: --remote requires --cookie-file with an existing paired console session")
	}
	cookie, err := os.ReadFile(r.CookieFile)
	if err != nil {
		return nil, fmt.Errorf("shape: read session cookie: %w", err)
	}
	value := strings.TrimSpace(string(cookie))
	if value == "" || strings.ContainsAny(value, "\r\n;") {
		return nil, fmt.Errorf("shape: cookie file must contain one session cookie value")
	}
	method, path, body, err := shapeRemoteOperation(r)
	if err != nil {
		return nil, err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/shape/" + url.PathEscape(r.App) + path
	if r.Verb == "suggest" {
		q := u.Query()
		q.Set("page", r.Page)
		u.RawQuery = q.Encode()
	}
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), payload)
	if err != nil {
		return nil, err
	}
	sessionCookie := &http.Cookie{Name: "bashy_console", Value: value}
	if err := sessionCookie.Valid(); err != nil {
		return nil, fmt.Errorf("shape: invalid session cookie value")
	}
	req.AddCookie(sessionCookie)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 25 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("shape: remote request failed: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Only a structured verification refusal is inspectable. Never render a
		// login page or arbitrary authentication error body as a candidate.
		if resp.StatusCode == http.StatusConflict && (r.Verb == "verify" || r.Verb == "approve") {
			var candidate map[string]any
			if json.Unmarshal(b, &candidate) == nil && candidate["id"] == r.ID {
				if verdict, ok := candidate["verdict"].(map[string]any); ok && verdict["pass"] == false {
					return candidate, fmt.Errorf("shape: verification did not pass (HTTP %d)", resp.StatusCode)
				}
			}
		}
		return nil, fmt.Errorf("shape: remote operation refused (HTTP %d)", resp.StatusCode)
	}
	var result any
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("shape: remote returned invalid JSON")
	}
	if r.Verb == "diff" {
		if patch, ok := result.(map[string]any); ok {
			if text, ok := patch["diff"].(string); ok {
				return text, nil
			}
		}
		return nil, fmt.Errorf("shape: remote did not return a raw diff")
	}
	if r.Verb == "verify" {
		c, ok := result.(map[string]any)
		if !ok {
			return result, fmt.Errorf("shape: missing verifier verdict")
		}
		v, ok := c["verdict"].(map[string]any)
		if !ok || v["pass"] != true {
			return result, fmt.Errorf("shape: verification did not pass")
		}
	}
	return result, nil
}

func shapeRemoteOperation(r shapeCLIRequest) (string, string, any, error) {
	id := url.PathEscape(r.ID)
	switch r.Verb {
	case "status":
		return "GET", "", nil, nil
	case "wish":
		if r.From != "" {
			break
		}
		body := map[string]string{"request": r.Request, "page": r.Page, "element": r.Element}
		if r.Story != "" {
			body["story"] = r.Story
		}
		if r.Todo != "" {
			body["todo"] = r.Todo
		}
		return "POST", "/candidates", body, nil
	case "show":
		return "GET", "/candidates/" + id, nil, nil
	case "diff":
		return "GET", "/candidates/" + id + "/diff", nil, nil
	case "verify", "approve", "reject":
		return "POST", "/candidates/" + id + "/" + r.Verb, map[string]any{}, nil
	case "incident":
		if r.Request != "" {
			return "POST", "/incidents", map[string]string{"kind": r.IncidentKind, "path": r.IncidentPath, "message": r.Request}, nil
		}
		if r.ID != "" {
			return "GET", "/incidents/" + id, nil, nil
		}
	case "suggest":
		return "GET", "/suggest", nil, nil
	case "history":
		return "GET", "/history", nil, nil
	case "undo":
		return "POST", "/history/undo", map[string]any{}, nil
	case "restore":
		return "POST", "/history/restore", map[string]string{"rev": r.ID}, nil
	}
	return "", "", nil, fmt.Errorf("shape: %s has no supported remote API mapping in this candidate", r.Verb)
}
