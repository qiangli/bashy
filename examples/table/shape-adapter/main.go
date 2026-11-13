package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed view.css
var viewCSS string

// runtimeSource accepts only a principal's private shaped data directory.
// The immutable, community-only runtime is shared; its databases are not.
func runtimeSource(data string) (string, error) {
	abs, err := filepath.Abs(data)
	if err != nil { return "", err }
	principal := filepath.Dir(abs)
	app := filepath.Dir(principal)
	shape := filepath.Dir(app)
	if filepath.Base(abs) != "data" || filepath.Base(app) != "table" || filepath.Base(shape) != "shape" {
		return "", errors.New("a paired table principal is required")
	}
	return filepath.Join(filepath.Dir(shape), "table", "runtime"), nil
}

func main() {
	if len(os.Args) != 3 { log.Fatal("usage: server ADDR DATA") }
	addr, data := os.Args[1], os.Args[2]
	data, err := filepath.Abs(data)
	if err != nil { log.Fatal(err) }
	source, err := runtimeSource(data)
	if err != nil { log.Fatal(err) }
	if _, err := os.Stat(filepath.Join(source, "_build", "stubs", "app", "server", "server.js")); err != nil {
		log.Fatalf("table runtime missing: %v", err)
	}
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { log.Fatal(err) }
	innerAddr := inner.Addr().(*net.TCPAddr)
	port := innerAddr.Port
	_ = inner.Close()
	if err := os.MkdirAll(filepath.Join(data, "docs"), 0700); err != nil { log.Fatal(err) }
	node := "/usr/bin/node"
	if _, err := os.Stat(node); err != nil { node = "/usr/local/bin/node" }
	cmd := exec.Command(node, filepath.Join(source, "_build", "stubs", "app", "server", "server.js"))
	cmd.Dir = source
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", port), "GRIST_HOST=127.0.0.1", "GRIST_EDITION=community",
		"GRIST_IN_SERVICE=true", "GRIST_TELEMETRY_LEVEL=off",
		"GRIST_ALLOW_AUTOMATIC_VERSION_CHECKING=false", "GRIST_SANDBOX_FLAVOR=unsandboxed",
		"GRIST_SKIP_EXT_AUTOSETUP=1",
		"TYPEORM_DATABASE="+filepath.Join(data, "landing.db"),
		"GRIST_DATA_DIR="+filepath.Join(data, "docs"),
		"NODE_PATH="+strings.Join([]string{filepath.Join(source,"_build"), filepath.Join(source,"_build","ext"), filepath.Join(source,"_build","stubs"), filepath.Join(source,"ext","node_modules")}, ":"),
	)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil { log.Fatal(err) }
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	upstream := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	direct := proxy.Director
	proxy.Director = func(r *http.Request) { direct(r); r.Header.Del("Accept-Encoding") }
	proxy.ModifyResponse = func(r *http.Response) error {
		if !strings.Contains(r.Header.Get("Content-Type"), "text/html") { return nil }
		body, err := io.ReadAll(r.Body)
		if err != nil { return err }
		_ = r.Body.Close()
		body = bytes.ReplaceAll(body, []byte(`href="/`), []byte(`href="/table/`))
		body = bytes.ReplaceAll(body, []byte(`src="/`), []byte(`src="/table/`))
		body = bytes.Replace(body, []byte("</head>"), []byte(`<link rel="stylesheet" href="/table/shape-view.css"></head>`), 1)
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.Header.Set("Content-Length", fmt.Sprint(len(body)))
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) { http.Error(w, "table runtime is starting", http.StatusServiceUnavailable) }
	mux := http.NewServeMux()
	mux.HandleFunc("/shape-view.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, viewCSS)
	})
	mux.Handle("/", proxy)
	server := &http.Server{Addr: addr, Handler: ownerTrace(mux, data), ReadHeaderTimeout: 5*time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-stop; _ = server.Close() }()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed { log.Fatal(err) }
}
