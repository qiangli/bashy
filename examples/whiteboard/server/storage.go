package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Storage struct {
	baseDir string
}

func NewStorage(dataDir string) (*Storage, error) {
	s := &Storage{baseDir: dataDir}
	if err := os.MkdirAll(filepath.Join(dataDir, "scenes"), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "files"), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "boards"), 0700); err != nil {
		return nil, err
	}
	return s, nil
}

// ServeHTTP handles storage API routes for Excalidraw scene persistence:
// POST /api/v2/post/ or /whiteboard/api/v2/post/ -> saves scene buffer, returns {"id": "<id>"}
// GET  /api/v2/{id}   or /whiteboard/api/v2/{id}   -> returns scene buffer
func (s *Storage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasPrefix(path, "/whiteboard") {
		path = strings.TrimPrefix(path, "/whiteboard")
	}

	switch {
	case r.Method == http.MethodPost && (path == "/api/v2/post" || path == "/api/v2/post/"):
		s.handlePostScene(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/api/v2/"):
		id := strings.TrimPrefix(path, "/api/v2/")
		id = strings.Trim(id, "/")
		s.handleGetScene(w, r, id)
	case strings.HasPrefix(path, "/files/"):
		s.handleFiles(w, r, strings.TrimPrefix(path, "/files/"))
	default:
		http.NotFound(w, r)
	}
}

func (s *Storage) handlePostScene(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 100*1024*1024))
	if err != nil || len(body) == 0 {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)

	filePath := filepath.Join(s.baseDir, "scenes", id)
	if err := os.WriteFile(filePath, body, 0600); err != nil {
		http.Error(w, "failed to save scene", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (s *Storage) handleGetScene(w http.ResponseWriter, r *http.Request, id string) {
	if id == "" || strings.Contains(id, "..") || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	filePath := filepath.Join(s.baseDir, "scenes", id)
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Storage) handleFiles(w http.ResponseWriter, r *http.Request, relPath string) {
	clean := filepath.Clean(relPath)
	if clean == "." || strings.HasPrefix(clean, "..") || strings.Contains(clean, "..") {
		http.NotFound(w, r)
		return
	}
	target := filepath.Join(s.baseDir, "files", clean)

	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, 50*1024*1024))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method == http.MethodGet {
		data, err := os.ReadFile(target)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
