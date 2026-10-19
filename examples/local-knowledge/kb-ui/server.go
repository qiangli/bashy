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
		preview, e := adapter.ReadSource(r.Context(), r.URL.Query().Get("uri"))
		if e != nil {
			http.Error(w, "source unavailable or excluded", http.StatusForbidden)
			return
		}
		content := []byte(preview.Body)
		if len(content) > maxPreviewBytes {
			http.Error(w, "preview exceeds limit", http.StatusRequestEntityTooLarge)
			return
		}
		sum := sha256.Sum256(content)
		w.Header().Set("X-Kg-Bytes", fmt.Sprint(len(content)))
		w.Header().Set("X-Kg-Sha256", hex.EncodeToString(sum[:]))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		if r.URL.Query().Get("download") == "1" {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", preview.Title+".txt"))
		}
		_, _ = w.Write(content)
	}
}
func handleMarkdownPreview(adapter Adapter) http.HandlerFunc { return handleFilePreview(adapter) }

func handleNodeDetail(adapter Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
		id = strings.TrimSpace(id)
		if id == "" {
			http.Error(w, "Node ID required", http.StatusBadRequest)
			return
		}

		source, err := adapter.ReadSource(r.Context(), id)
		if err != nil {
			http.Error(w, "indexed source unavailable", http.StatusNotFound)
			return
		}
		node := map[string]any{"node_id": source.ID, "node_type": "Document", "label": source.Title, "source_uri": source.Source.Ref, "text": source.Body, "updated_at": source.Updated}

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
			if _, err := adapter.IndexStatus(r.Context()); err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
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

			if len(req.Includes) > 0 || len(req.Excludes) > 0 {
				http.Error(w, "configure exclusions through the shared index policy", http.StatusBadRequest)
				return
			}
			resolved, err := ResolvePath(req.Path)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err = adapter.IndexAdd(r.Context(), resolved); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			var cfg RootConfig
			for _, root := range rm.List() {
				if root.Resolved == resolved {
					cfg = root
					break
				}
			}

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

func handleDisk(Adapter) http.HandlerFunc {
	return handleUnsupported("disk telemetry is unavailable in the shared index adapter")
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
		st, e := adapter.IndexStatus(r.Context())
		if e != nil {
			http.Error(w, e.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"roots": st.Roots, "total_emitted": st.DocumentCount, "finished_all": st.State == "ready", "state": st.State, "last_sync": st.LastSync})
	}
}

func handleSubstagesLive(_ Adapter) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		// Return 503 so monitor.js displays "substages telemetry not configured" cleanly
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"substages telemetry not configured in local-knowledge adapter"}`))
	}
}

func handleGraphQuery(Adapter) http.HandlerFunc {
	return handleUnsupported("graph enrichment is unavailable; indexed search retains source provenance")
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
