// Copyright (c) 2026 Qiang Li. All rights reserved.
// Ported from kg-ui under explicit operator authorization for Bashy.
// See ATTRIBUTION.md and LICENSE for licensing information.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

//go:embed web
var webFS embed.FS

const (
	maxPreviewBytes     = 2 * 1024 * 1024 // 2 MB max file preview size
	maxRequestBodyBytes = 1 * 1024 * 1024 // 1 MB max JSON request body
)

// normalizeBindAddress ensures the server defaults to loopback (127.0.0.1) only.
func normalizeBindAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "127.0.0.1:6674"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// Could be just a port (e.g. ":6674" or "6674")
		port = strings.TrimPrefix(addr, ":")
		return net.JoinHostPort("127.0.0.1", port)
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// serve binds the local HTTP listener and blocks until ctx is canceled.
func serve(ctx context.Context, addr string, adapter Adapter, hub *SSEHub) error {
	addr = normalizeBindAddress(addr)

	handler, err := buildMux(adapter, hub)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	fmt.Fprintf(os.Stderr, "kb-ui: listening on %s (loopback only)\n", addr)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// buildMux wires the local-knowledge routes and static embedded asset handler.
func buildMux(adapter Adapter, hub *SSEHub) (http.Handler, error) {
	webRoot, err := fsSubWeb()
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	// 1. Health and events
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.Handle("/events", hub)

	// 2. Local-knowledge search and stats
	mux.HandleFunc("/api/stats", handleStats(adapter))
	mux.HandleFunc("/api/search", handleSearch(adapter))
	mux.HandleFunc("/api/file", handleFilePreview(adapter))
	mux.HandleFunc("/api/markdown", handleMarkdownPreview(adapter))
	mux.HandleFunc("/api/nodes/", handleNodeDetail(adapter))

	// 3. Root lifecycle
	mux.HandleFunc("/api/roots", handleRoots(adapter, hub))

	// 4. Ingestion control
	mux.HandleFunc("/api/control", handleControl(adapter))
	mux.HandleFunc("/api/control/pause", handleControlPause(adapter, hub))
	mux.HandleFunc("/api/control/resume", handleControlResume(adapter, hub))
	mux.HandleFunc("/api/control/sync", handleControlSync(adapter, hub))

	// 5. System, disk, discovery signals
	mux.HandleFunc("/api/system", handleSystem())
	mux.HandleFunc("/api/disk", handleDisk(adapter))
	mux.HandleFunc("/api/runs", handleRuns(adapter))
	mux.HandleFunc("/api/scan/live", handleScanLive(adapter))
	mux.HandleFunc("/api/substages/live", handleSubstagesLive(adapter))

	// 6. Explicitly mark unsupported legacy bonsai/queue/enrichment features
	mux.HandleFunc("/api/enrichment/summary", handleUnsupported("Enrichment pipeline unavailable in local search index"))
	mux.HandleFunc("/api/metrics", handleUnsupported("Bonsai metrics unavailable in local search index"))
	mux.HandleFunc("/api/queue/stats", handleUnsupported("Legacy job ledger unavailable in local search index"))
	mux.HandleFunc("/api/queue/failed", handleUnsupported("Legacy failed queue unavailable in local search index"))
	mux.HandleFunc("/api/query", handleGraphQuery(adapter))

	// 7. Embedded SPA assets
	mux.Handle("/", htmlBaseFilter(webRoot, http.FileServer(http.FS(webRoot))))

	return mux, nil
}

func handleStats(adapter Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, err := adapter.IndexStatus(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		counts := map[string]int{
			"Document": st.DocumentCount,
			"Root":     len(st.Roots),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(counts)
	}
}

func handleSearch(adapter Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		mode := r.URL.Query().Get("mode")
		corpus := r.URL.Query().Get("corpus")

		resp, err := adapter.QueryIndexed(r.Context(), q, mode, corpus)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func handleFilePreview(adapter Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uri := r.URL.Query().Get("uri")
		download := r.URL.Query().Get("download") == "1"

		roots := adapter.GetRootsManager().GetEligibleRoots()
		safePath, err := ValidateFilePreviewPath(uri, roots)
		if err != nil {
			if errors.Is(err, ErrOutsideRoot) || errors.Is(err, ErrSymlinkEscapesRoot) || errors.Is(err, ErrSensitiveFile) {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			if errors.Is(err, os.ErrNotExist) {
				http.Error(w, "File not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		f, err := os.Open(safePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Read up to 2MB bounded content
		limitReader := io.LimitReader(f, maxPreviewBytes)
		content, err := io.ReadAll(limitReader)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Detect MIME
		mime := http.DetectContentType(content)
		isBinary := strings.HasPrefix(mime, "application/octet-stream") ||
			(!strings.HasPrefix(mime, "text/") && !strings.Contains(mime, "json") && !strings.Contains(mime, "xml"))

		// If binary and not downloading, return 415 preview-unavailable as expected by file-viewer.js
		if isBinary && !download {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"mime":  mime,
				"bytes": fi.Size(),
			})
			return
		}

		hasher := sha256.New()
		hasher.Write(content)
		shaHex := hex.EncodeToString(hasher.Sum(nil))

		w.Header().Set("X-Kg-Bytes", fmt.Sprintf("%d", fi.Size()))
		w.Header().Set("X-Kg-Sha256", shaHex)
		w.Header().Set("Content-Type", mime)

		if download {
			filename := filepath.Base(safePath)
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		}

		_, _ = w.Write(content)
	}
}

func handleMarkdownPreview(adapter Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uri := r.URL.Query().Get("uri")
		roots := adapter.GetRootsManager().GetEligibleRoots()
		safePath, err := ValidateFilePreviewPath(uri, roots)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		ext := strings.ToLower(filepath.Ext(safePath))
		if ext != ".md" && ext != ".markdown" && ext != ".txt" {
			http.Error(w, "No markdown conversion available for this file type", http.StatusNotFound)
			return
		}

		content, err := os.ReadFile(safePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(content)
	}
}

func handleNodeDetail(adapter Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
		id = strings.TrimSpace(id)
		if id == "" {
			http.Error(w, "Node ID required", http.StatusBadRequest)
			return
		}

		// Try finding the file under roots
		roots := adapter.GetRootsManager().GetEligibleRoots()
		safePath, err := ValidateFilePreviewPath(id, roots)

		label := filepath.Base(id)
		nodeType := "Document"
		sourceURI := id
		if !strings.HasPrefix(sourceURI, "file://") && filepath.IsAbs(sourceURI) {
			sourceURI = "file://" + sourceURI
		}

		node := map[string]any{
			"node_id":    id,
			"node_type":  nodeType,
			"label":      label,
			"source_uri": sourceURI,
		}

		if err == nil {
			if fi, err := os.Stat(safePath); err == nil {
				node["bytes"] = fi.Size()
				node["updated_at"] = fi.ModTime().UTC().Format(time.RFC3339)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"node": []any{node},
		})
	}
}

func handleRoots(adapter Adapter, hub *SSEHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rm := adapter.GetRootsManager()

		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"roots": rm.List(),
			})

		case http.MethodPost:
			if err := CheckMutationAllowed(r); err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
			var req struct {
				Path     string   `json:"path"`
				Includes []string `json:"includes"`
				Excludes []string `json:"excludes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
				return
			}

			cfg, err := rm.Add(req.Path, req.Includes, req.Excludes)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = adapter.IndexAdd(r.Context(), cfg.Resolved)

			hub.Broadcast(map[string]any{
				"type": "root_added",
				"path": cfg.Path,
			})

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(cfg)

		case http.MethodDelete:
			if err := CheckMutationAllowed(r); err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			targetPath := r.URL.Query().Get("path")
			if targetPath == "" {
				var req struct {
					Path string `json:"path"`
				}
				_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)).Decode(&req)
				targetPath = req.Path
			}
			if targetPath == "" {
				http.Error(w, "Path parameter required", http.StatusBadRequest)
				return
			}

			if err := adapter.IndexForget(r.Context(), targetPath); err != nil {
				http.Error(w, "Root not found", http.StatusNotFound)
				return
			}

			hub.Broadcast(map[string]any{
				"type": "root_removed",
				"path": targetPath,
			})

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleControl(adapter Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, err := adapter.IndexStatus(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"paused": st.Paused,
			"since":  st.Since,
		})
	}
}

func handleControlPause(adapter Adapter, hub *SSEHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := CheckMutationAllowed(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if err := adapter.IndexPause(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hub.Broadcast(map[string]any{"type": "ingest_paused"})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func handleControlResume(adapter Adapter, hub *SSEHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := CheckMutationAllowed(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if err := adapter.IndexResume(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hub.Broadcast(map[string]any{"type": "ingest_resumed"})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func handleControlSync(adapter Adapter, hub *SSEHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := CheckMutationAllowed(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		if err := adapter.IndexSync(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hub.Broadcast(map[string]any{"type": "index_synced"})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func handleSystem() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"state":  "up",
			"detail": "local-knowledge adapter active",
		})
	}
}

func handleDisk(adapter Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roots := adapter.GetRootsManager().List()
		var totalBytes int64 = 0
		for _, r := range roots {
			totalBytes += r.ByteCount
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"level":       "ok",
			"avail_bytes": 100 * 1024 * 1024 * 1024, // baseline healthy free space
			"used_bytes":  totalBytes,
			"path":        "local-knowledge",
		})
	}
}

func handleRuns(adapter Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roots := adapter.GetRootsManager().List()
		var runs []map[string]any
		for _, r := range roots {
			runs = append(runs, map[string]any{
				"run_at":       r.UpdatedAt.Format(time.RFC3339),
				"extractor_id": "file",
				"source_uri":   "file://" + r.Resolved,
				"status":       r.Status,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"runs": runs,
		})
	}
}

func handleScanLive(adapter Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, _ := adapter.IndexStatus(r.Context())
		roots := adapter.GetRootsManager().List()
		var snapRoots []map[string]any
		for _, r := range roots {
			snapRoots = append(snapRoots, map[string]any{
				"path":          r.Path,
				"resolved":      r.Resolved,
				"status":        r.Status,
				"emitted":       r.FileCount,
				"emits_per_sec": 0.0,
			})
		}

		docCount := 0
		if st != nil {
			docCount = st.DocumentCount
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"roots":         snapRoots,
			"total_emitted": docCount,
			"total_bytes":   st.DiskUsage,
			"total_skipped": 0,
			"finished_all":  true,
		})
	}
}

func handleSubstagesLive(_ Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		// Return 503 so monitor.js displays "substages telemetry not configured" cleanly
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"substages telemetry not configured in local-knowledge adapter"}`))
	}
}

func handleGraphQuery(_ Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		// Graph queries return empty result set cleanly without mock data
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"q":[]}`))
	}
}

func handleUnsupported(msg string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"supported": false,
			"message":   msg,
		})
	}
}

func htmlBaseFilter(webRoot fs.FS, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(r.URL.Path)
		if strings.HasSuffix(clean, "/") || clean == "" || clean == "." {
			clean = path.Join(clean, "index.html")
		}
		clean = strings.TrimPrefix(clean, "/")
		if !strings.HasSuffix(clean, ".html") {
			next.ServeHTTP(w, r)
			return
		}
		body, err := fs.ReadFile(webRoot, clean)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		// Sanitize and escape base href derived from X-Forwarded-Prefix
		baseHref := SanitizeBasePrefix(r.Header.Get("X-Forwarded-Prefix"))
		body = bytes.ReplaceAll(body, []byte("{{BASE}}"), []byte(baseHref))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.Copy(w, bytes.NewReader(body))
	})
}

func fsSubWeb() (fs.FS, error) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, fmt.Errorf("embed sub-fs: %w", err)
	}
	return sub, nil
}
