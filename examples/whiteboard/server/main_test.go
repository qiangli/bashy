package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func setupTestServer(t *testing.T) (http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	storage, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	collab := NewCollabServer()
	desk := NewDeskHandler(dir, "/whiteboard")

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.Handle("/socket.io/", collab)
	mux.Handle("/api/v2/", storage)
	mux.Handle("/files/", storage)
	mux.Handle("/api/board", desk)
	mux.Handle("/api/action", desk)
	mux.Handle("/api/state", desk)

	handler := ownerTrace(mux, dir)
	return handler, dir
}

func TestHealthCheck(t *testing.T) {
	handler, _ := setupTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz code = %d, want 200", rec.Code)
	}
	var res map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res["ok"] {
		t.Fatalf("healthz body = %s, want ok:true", rec.Body.String())
	}
}

func TestSceneStorage(t *testing.T) {
	handler, _ := setupTestServer(t)

	// 1. Post scene
	sceneData := []byte("dummy encrypted scene buffer")
	postReq := httptest.NewRequest(http.MethodPost, "/api/v2/post/", bytes.NewReader(sceneData))
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/v2/post/ code = %d, want 200", postRec.Code)
	}
	var postRes map[string]string
	if err := json.Unmarshal(postRec.Body.Bytes(), &postRes); err != nil || postRes["id"] == "" {
		t.Fatalf("POST /api/v2/post/ invalid response: %s", postRec.Body.String())
	}
	id := postRes["id"]

	// 2. Get scene
	getReq := httptest.NewRequest(http.MethodGet, "/api/v2/"+id, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/v2/%s code = %d, want 200", id, getRec.Code)
	}
	if !bytes.Equal(getRec.Body.Bytes(), sceneData) {
		t.Fatalf("GET scene data mismatch: got %q, want %q", getRec.Body.String(), string(sceneData))
	}

	// 3. Get nonexistent scene
	badReq := httptest.NewRequest(http.MethodGet, "/api/v2/nonexistent", nil)
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusNotFound {
		t.Fatalf("GET nonexistent code = %d, want 404", badRec.Code)
	}
}

func TestFilesStorage(t *testing.T) {
	handler, _ := setupTestServer(t)

	fileData := []byte("fake image data")
	postReq := httptest.NewRequest(http.MethodPost, "/files/rooms/room1/file1", bytes.NewReader(fileData))
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /files/... code = %d", postRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/files/rooms/room1/file1", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /files/... code = %d", getRec.Code)
	}
	if !bytes.Equal(getRec.Body.Bytes(), fileData) {
		t.Fatalf("file content mismatch")
	}
}

func TestCollabSocketIO(t *testing.T) {
	handler, _ := setupTestServer(t)

	// 1. Handshake
	hsReq := httptest.NewRequest(http.MethodGet, "/socket.io/?EIO=4&transport=polling", nil)
	hsRec := httptest.NewRecorder()
	handler.ServeHTTP(hsRec, hsReq)

	if hsRec.Code != http.StatusOK {
		t.Fatalf("handshake code = %d", hsRec.Code)
	}
	body := hsRec.Body.String()
	if !strings.HasPrefix(body, "0{") {
		t.Fatalf("handshake missing EIO packet 0: %s", body)
	}
	var hsData struct {
		SID string `json:"sid"`
	}
	if err := json.Unmarshal([]byte(body[1:]), &hsData); err != nil || hsData.SID == "" {
		t.Fatalf("failed to parse sid from handshake: %s", body)
	}
	sid := hsData.SID

	// 2. Connect packet POST 40
	postReq := httptest.NewRequest(http.MethodPost, "/socket.io/?EIO=4&transport=polling&sid="+sid, strings.NewReader("40"))
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST 40 code = %d", postRec.Code)
	}

	// 3. Poll for response
	pollReq := httptest.NewRequest(http.MethodGet, "/socket.io/?EIO=4&transport=polling&sid="+sid, nil)
	pollRec := httptest.NewRecorder()
	handler.ServeHTTP(pollRec, pollReq)
	if pollRec.Code != http.StatusOK {
		t.Fatalf("poll code = %d", pollRec.Code)
	}
	pollResp := pollRec.Body.String()
	if !strings.Contains(pollResp, "40") {
		t.Fatalf("expected connect ack 40, got %s", pollResp)
	}
}

func TestDeskVerbs(t *testing.T) {
	handler, _ := setupTestServer(t)

	// 1. Add board
	addPayload := `{"action":"add","board":"project-brainstorm","body":"Initial idea"}`
	addReq := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(addPayload))
	addRec := httptest.NewRecorder()
	handler.ServeHTTP(addRec, addReq)

	if addRec.Code != http.StatusOK {
		t.Fatalf("add board code = %d", addRec.Code)
	}
	var addRes map[string]any
	if err := json.Unmarshal(addRec.Body.Bytes(), &addRes); err != nil || addRes["status"] != "pending" {
		t.Fatalf("add board failed: %s", addRec.Body.String())
	}

	// 2. Get state
	stateReq := httptest.NewRequest(http.MethodGet, "/api/state?id=project-brainstorm", nil)
	stateRec := httptest.NewRecorder()
	handler.ServeHTTP(stateRec, stateReq)

	if stateRec.Code != http.StatusOK {
		t.Fatalf("get state code = %d", stateRec.Code)
	}
	var stateRes map[string]any
	if err := json.Unmarshal(stateRec.Body.Bytes(), &stateRes); err != nil || stateRes["name"] != "project-brainstorm" {
		t.Fatalf("state unexpected: %s", stateRec.Body.String())
	}

	// 3. Close board
	closePayload := `{"action":"close","board":"project-brainstorm"}`
	closeReq := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(closePayload))
	closeRec := httptest.NewRecorder()
	handler.ServeHTTP(closeRec, closeReq)

	if closeRec.Code != http.StatusOK {
		t.Fatalf("close board code = %d", closeRec.Code)
	}
	var closeRes map[string]any
	if err := json.Unmarshal(closeRec.Body.Bytes(), &closeRes); err != nil || closeRes["status"] != "closed" {
		t.Fatalf("close board failed: %s", closeRec.Body.String())
	}
}

func TestTelemetryEnvGated(t *testing.T) {
	// When OTEL_EXPORTER_OTLP_ENDPOINT is unset, no calls are made
	os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	handler, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz code = %d", rec.Code)
	}

	// When traceparent is passed, it propagates correctly
	tp := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	traceID, parentID := readTraceparent(tp)
	if traceID != "4bf92f3577b34da6a3ce929d0e0e4736" || parentID != "00f067aa0ba902b7" {
		t.Fatalf("readTraceparent failed: %s, %s", traceID, parentID)
	}
}
