package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func findStaticDir() string {
	// Look for excalidraw-app/build in working dir, app root, or parent dirs
	candidates := []string{
		"excalidraw-app/build",
		"../excalidraw-app/build",
		"../../excalidraw-app/build",
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "excalidraw-app", "build"),
			filepath.Join(dir, "..", "excalidraw-app", "build"),
		)
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err == nil {
			if st, err := os.Stat(filepath.Join(abs, "index.html")); err == nil && !st.IsDir() {
				return abs
			}
		}
	}
	return ""
}

func main() {
	var (
		addr   = flag.String("addr", "127.0.0.1:18093", "listen address")
		port   = flag.Int("port", 0, "listen port (overrides addr port if non-zero)")
		data   = flag.String("data", "", "data directory")
		prefix = flag.String("prefix", "/whiteboard", "URL prefix")
	)
	flag.Parse()

	if *port != 0 {
		host, _, err := net.SplitHostPort(*addr)
		if err != nil {
			host = "127.0.0.1"
		}
		*addr = net.JoinHostPort(host, strconv.Itoa(*port))
	}

	dataDir := *data
	if dataDir == "" {
		xdg := os.Getenv("XDG_DATA_HOME")
		if xdg == "" {
			xdg = filepath.Join(os.Getenv("HOME"), ".local", "share")
		}
		dataDir = filepath.Join(xdg, "bashy", "whiteboard")
	}
	dataDir, _ = filepath.Abs(dataDir)

	storage, err := NewStorage(dataDir)
	if err != nil {
		log.Fatalf("failed to initialize storage: %v", err)
	}

	collab := NewCollabServer()
	desk := NewDeskHandler(dataDir, *prefix)
	staticDir := findStaticDir()

	mux := http.NewServeMux()

	// 1. Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/whiteboard/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	// 2. Collab (Socket.IO polling)
	mux.Handle("/socket.io/", collab)
	mux.Handle("/whiteboard/socket.io/", collab)

	// 3. Scene and file storage
	mux.Handle("/api/v2/", storage)
	mux.Handle("/whiteboard/api/v2/", storage)
	mux.Handle("/files/", storage)
	mux.Handle("/whiteboard/files/", storage)

	// 4. Desk verbs (folding in whiteboard and brainstorm)
	mux.Handle("/api/board", desk)
	mux.Handle("/whiteboard/api/board", desk)
	mux.Handle("/api/action", desk)
	mux.Handle("/whiteboard/api/action", desk)
	mux.Handle("/api/state", desk)
	mux.Handle("/whiteboard/api/state", desk)

	// 5. Static files & SPA fallback
	fileServer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/whiteboard") {
			p = strings.TrimPrefix(p, "/whiteboard")
		}
		if p == "" || p == "/" {
			p = "/index.html"
		}

		if staticDir != "" {
			filePath := filepath.Join(staticDir, filepath.Clean(p))
			if st, err := os.Stat(filePath); err == nil && !st.IsDir() {
				http.ServeFile(w, r, filePath)
				return
			}
			// Fallback to index.html for SPA routes
			indexPath := filepath.Join(staticDir, "index.html")
			if _, err := os.Stat(indexPath); err == nil {
				http.ServeFile(w, r, indexPath)
				return
			}
		}

		// Fallback minimal HTML if staticDir not yet built
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, "<!doctype html><html><head><title>Whiteboard</title></head><body><h1>Whiteboard</h1><p>Data: %s</p></body></html>", dataDir)
	})

	mux.Handle("/", fileServer)

	handler := ownerTrace(mux, dataDir)

	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-stop
		_ = server.Close()
	}()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen error: %v", err)
	}

	fmt.Printf("Whiteboard listening on http://%s\n", ln.Addr().String())

	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
