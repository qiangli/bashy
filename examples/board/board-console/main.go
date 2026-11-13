// Command board-console is the shaped server entry for the board app.
//
// It listens on -addr immediately (the console requires a listener within
// seconds), then brings up a per-data-directory Postgres cluster and the
// prebuilt board API in the background. Until the API answers, /board/api/*
// returns 503; the static web app under /board/ is served from the start.
//
// The API bundle, its drizzle migrations and the web build are embedded at
// compile time from the adapter commit, so the binary needs only `node` and
// a Postgres toolkit on PATH at run time (found via well-known locations).
// Nothing is sent off the machine: vendor telemetry was removed from the
// shipped bundle at install time (see the board README), and the only
// network listeners are loopback.
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

//go:embed dist/api.mjs
var apiBundle string

//go:embed dist/drizzle
//go:embed dist/drizzle/meta/_journal.json
var drizzleFS embed.FS

//go:embed web
var webFS embed.FS

// distStamp identifies the embedded build; the install writes it from the
// pinned source revision so a newer binary refreshes the on-disk files.
// The API bundle lives in dist/ so the migrator's `${bundleDir}/../drizzle`
// lookup lands on dist/../drizzle.
const distStamp = "board-dist-v3:5069fb56ce309d5572ca3ed60d4d3c322a03a452"

var (
	addrFlag = flag.String("addr", "127.0.0.1:22751", "loopback host:port to listen on")
	dataFlag = flag.String("data", "", "principal data directory (required)")
	apiReady atomic.Bool
)

func main() {
	flag.Parse()
	if *dataFlag == "" {
		log.Fatal("board-console: -data is required")
	}
	if err := os.MkdirAll(*dataFlag, 0o700); err != nil {
		log.Fatalf("board-console: data dir: %v", err)
	}

	ln, err := net.Listen("tcp", *addrFlag)
	if err != nil {
		log.Fatalf("board-console: listen %s: %v", *addrFlag, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/board/api" {
			http.Redirect(w, r, "/board/api/", http.StatusFound)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/board/api/") {
			if !apiReady.Load() {
				http.Error(w, "board api is starting", http.StatusServiceUnavailable)
				return
			}
			apiProxy.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/board/", http.StatusFound)
			return
		}
		if r.URL.Path == "/board" {
			http.Redirect(w, r, "/board/", http.StatusFound)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/board/") {
			http.NotFound(w, r)
			return
		}
		serveWeb(w, r)
	})
	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("board-console: serve: %v", err)
		}
	}()

	proc, err := bringUp(*dataFlag)
	if err != nil {
		log.Printf("board-console: backend failed: %v", err)
		_ = srv.Close()
		os.Exit(1)
	}
	apiReady.Store(true)
	log.Printf("board-console: serving board on %s (data %s)", *addrFlag, *dataFlag)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
	log.Print("board-console: shutting down")
	_ = srv.Close()
	proc.stop()
}

// apiProxy forwards /board/api/* to the bundled API with the /board prefix
// stripped, so the API sees the /api/* paths it was built with. WebSockets
// (live board updates) are proxied transparently.
var apiProxy = &httputil.ReverseProxy{
	Director: func(req *http.Request) {
		req.URL.Scheme = "http"
		req.URL.Host = apiAddr
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/board")
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
	},
}

var apiAddr string

func serveWeb(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/board/")
	if rest == "" {
		rest = "index.html"
	}
	data, err := webFS.ReadFile("web/" + rest)
	if err != nil {
		data, err = webFS.ReadFile("web/index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		rest = "index.html"
	}
	switch {
	case strings.HasSuffix(rest, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case strings.HasSuffix(rest, ".js"):
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case strings.HasSuffix(rest, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(rest, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	case strings.HasSuffix(rest, ".json"):
		w.Header().Set("Content-Type", "application/json")
	case strings.HasSuffix(rest, ".png"):
		w.Header().Set("Content-Type", "image/png")
	case strings.HasSuffix(rest, ".ico"):
		w.Header().Set("Content-Type", "image/x-icon")
	}
	_, _ = w.Write(data)
}

type backend struct {
	node *exec.Cmd
	pg   pgCluster
}

func (b *backend) stop() {
	if b.node != nil && b.node.Process != nil {
		_ = b.node.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = b.node.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = b.node.Process.Kill()
			<-done
		}
	}
	b.pg.stop()
}

// bringUp starts Postgres, then node, and waits for the API to answer.
func bringUp(data string) (*backend, error) {
	nodeBin, err := lookPath("node", extraBinDirs())
	if err != nil {
		return nil, fmt.Errorf("node not found: %w (install a Node.js runtime and retry)", err)
	}
	pg, err := startPostgres(data)
	if err != nil {
		return nil, err
	}
	files, err := materialize(data)
	if err != nil {
		pg.stop()
		return nil, err
	}
	apiPort, err := freePort()
	if err != nil {
		pg.stop()
		return nil, err
	}
	apiAddr = fmt.Sprintf("127.0.0.1:%d", apiPort)
	secret, err := authSecret(data)
	if err != nil {
		pg.stop()
		return nil, err
	}
	env := []string{
		"PATH=" + strings.Join(append(extraBinDirs(), "/usr/bin", "/bin"), string(os.PathListSeparator)),
		"HOME=" + data,
		"TMPDIR=" + filepath.Join(data, "tmp"),
		"LANG=C.UTF-8",
		"NODE_ENV=production",
		"BOARD_API_PORT=" + fmt.Sprint(apiPort),
		"DATABASE_URL=" + pg.databaseURL(),
		"AUTH_SECRET=" + secret,
	}
	nodePath, err := filepath.EvalSymlinks(files.api)
	if err != nil {
		nodePath = files.api
	}
	cmd := exec.Command(nodeBin, nodePath)
	cmd.Env = env
	cmd.Dir = data
	out, err := os.OpenFile(filepath.Join(data, "api.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		pg.stop()
		return nil, fmt.Errorf("api log: %w", err)
	}
	defer out.Close()
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		pg.stop()
		return nil, fmt.Errorf("start api: %w", err)
	}
	b := &backend{node: cmd, pg: pg}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.Now().Add(6 * time.Minute)
	client := &http.Client{Timeout: 5 * time.Second}
	for {
		select {
		case err := <-exited:
			b.stop()
			tail := lastLines(filepath.Join(data, "api.log"), 20)
			return nil, fmt.Errorf("api exited (%v):\n%s", err, tail)
		default:
		}
		if time.Now().After(deadline) {
			b.stop()
			return nil, fmt.Errorf("api did not answer within 6 minutes")
		}
		resp, err := client.Get("http://" + apiAddr + "/api/instance/status")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return b, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
}

type serverFiles struct {
	api string
}

// materialize writes the embedded API bundle and migrations under
// data/server-files, refreshing them when the embedded stamp changes.
func materialize(data string) (*serverFiles, error) {
	root := filepath.Join(data, "server-files")
	stampPath := filepath.Join(root, ".stamp")
	if cur, err := os.ReadFile(stampPath); err == nil && string(cur) == distStamp {
		return &serverFiles{api: filepath.Join(root, "dist", "api.mjs")}, nil
	}
	tmp, err := os.MkdirTemp(data, "server-files.")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "dist"), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(tmp, "dist", "api.mjs"), []byte(apiBundle), 0o600); err != nil {
		return nil, err
	}
	if err := copyEmbedDir(drizzleFS, "dist/drizzle", filepath.Join(tmp, "drizzle")); err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".stamp"), []byte(distStamp), 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, root); err != nil {
		// Another boot won the race; its files carry the same stamp.
		if cur, rerr := os.ReadFile(stampPath); rerr == nil && string(cur) == distStamp {
			return &serverFiles{api: filepath.Join(root, "dist", "api.mjs")}, nil
		}
		return nil, fmt.Errorf("install server files: %w", err)
	}
	return &serverFiles{api: filepath.Join(root, "dist", "api.mjs")}, nil
}

func copyEmbedDir(fsys embed.FS, src, dst string) error {
	return fs.WalkDir(fsys, src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fsys.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func authSecret(data string) (string, error) {
	path := filepath.Join(data, "auth-secret")
	if cur, err := os.ReadFile(path); err == nil && len(cur) >= 32 {
		return strings.TrimSpace(string(cur)), nil
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(raw[:])
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}

type pgCluster struct {
	binDir string
	data   string
	port   int
}

func (c pgCluster) databaseURL() string {
	return fmt.Sprintf("postgresql://board@127.0.0.1:%d/board", c.port)
}

func startPostgres(data string) (pgCluster, error) {
	var pg pgCluster
	binDir, err := pgBinDir()
	if err != nil {
		return pg, fmt.Errorf("postgres toolkit not found: %w (install PostgreSQL and retry)", err)
	}
	pg = pgCluster{binDir: binDir, data: filepath.Join(data, "pg")}
	tool := func(name string) string { return filepath.Join(binDir, name) }
	if port, ok := probeCluster(pg.data, tool); ok {
		pg.port = port
		if err := ensureBoardDB(tool, pg); err != nil {
			return pg, err
		}
		return pg, nil
	}
	if _, err := os.Stat(filepath.Join(pg.data, "PG_VERSION")); os.IsNotExist(err) {
		if err := os.MkdirAll(pg.data, 0o700); err != nil {
			return pg, err
		}
		initdb := exec.Command(tool("initdb"), "-U", "board", "--auth=trust", "-E", "UTF8", "-D", pg.data)
		initdb.Env = []string{"PATH=" + binDir + ":/usr/bin:/bin", "HOME=" + data, "LANG=C.UTF-8"}
		if out, err := initdb.CombinedOutput(); err != nil {
			return pg, fmt.Errorf("initdb: %w\n%s", err, out)
		}
	} else if v, rerr := os.ReadFile(filepath.Join(pg.data, "PG_VERSION")); rerr == nil {
		if out, rerr := exec.Command(tool("postgres"), "--version").Output(); rerr == nil {
			ver := fieldsVersion(string(out))
			if ver != "" && !strings.HasPrefix(strings.TrimSpace(string(v)), ver) {
				return pg, fmt.Errorf("postgres cluster is version %s but the toolkit reports %s; refusing to touch it (move %s aside to reinit)",
					strings.TrimSpace(string(v)), ver, pg.data)
			}
		}
	}
	port, err := freePort()
	if err != nil {
		return pg, err
	}
	pg.port = port
	sock := filepath.Join(data, "pgsock")
	if err := os.MkdirAll(sock, 0o700); err != nil {
		return pg, err
	}
	if err := os.WriteFile(filepath.Join(pg.data, "port"), []byte(fmt.Sprint(port)), 0o600); err != nil {
		pg.stop()
		return pg, err
	}
	start := exec.Command(tool("pg_ctl"), "start", "-D", pg.data, "-l", filepath.Join(pg.data, "log"),
		"-o", fmt.Sprintf("-h 127.0.0.1 -p %d -k %s", port, sock))
	start.Env = []string{"PATH=" + binDir + ":/usr/bin:/bin", "HOME=" + data, "LANG=C.UTF-8"}
	if out, err := start.CombinedOutput(); err != nil {
		return pg, fmt.Errorf("pg_ctl start: %w\n%s", err, out)
	}
	if err := waitReady(tool, data, pg.port, 60*time.Second); err != nil {
		pg.stop()
		return pg, err
	}
	if err := ensureBoardDB(tool, pg); err != nil {
		pg.stop()
		return pg, err
	}
	return pg, nil
}

// probeCluster reuses a cluster this launcher started before (same data
// dir) when it still answers, so a node crash or console restart does not
// orphan a running Postgres the next boot cannot use.
func probeCluster(data string, tool func(string) string) (int, bool) {
	raw, err := os.ReadFile(filepath.Join(data, "port"))
	if err != nil {
		return 0, false
	}
	var port int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(raw)), "%d", &port); err != nil || port <= 0 {
		return 0, false
	}
	cmd := exec.Command(tool("psql"), "-h", "127.0.0.1", "-p", fmt.Sprint(port),
		"-U", "board", "-d", "postgres", "-c", "SELECT 1")
	cmd.Env = []string{"PATH=" + filepath.Dir(tool("psql")) + ":/usr/bin:/bin", "LANG=C.UTF-8"}
	if err := cmd.Run(); err != nil {
		return 0, false
	}
	return port, true
}

func waitReady(tool func(string) string, data string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		cmd := exec.Command(tool("psql"), "-h", "127.0.0.1", "-p", fmt.Sprint(port),
			"-U", "board", "-d", "postgres", "-c", "SELECT 1")
		cmd.Env = []string{"PATH=" + filepath.Dir(tool("psql")) + ":/usr/bin:/bin", "HOME=" + data, "LANG=C.UTF-8"}
		if err := cmd.Run(); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("postgres did not accept connections within %s", timeout)
		}
		time.Sleep(time.Second)
	}
}

func ensureBoardDB(tool func(string) string, pg pgCluster) error {
	cmd := exec.Command(tool("psql"), "-h", "127.0.0.1", "-p", fmt.Sprint(pg.port),
		"-U", "board", "-d", "postgres", "-c", "CREATE DATABASE board")
	cmd.Env = []string{"PATH=" + filepath.Dir(tool("psql")) + ":/usr/bin:/bin", "LANG=C.UTF-8"}
	if out, err := cmd.CombinedOutput(); err != nil &&
		!strings.Contains(string(out)+err.Error(), "already exists") {
		return fmt.Errorf("create database: %w\n%s", err, out)
	}
	return nil
}

func (c pgCluster) stop() {
	if c.binDir == "" || c.data == "" {
		return
	}
	cmd := exec.Command(filepath.Join(c.binDir, "pg_ctl"), "stop", "-D", c.data, "-m", "fast")
	cmd.Env = []string{"PATH=" + c.binDir + ":/usr/bin:/bin", "HOME=" + filepath.Dir(c.data), "LANG=C.UTF-8"}
	_ = cmd.Run()
}

// pgBinDir finds a Postgres toolkit (initdb, pg_ctl, postgres, psql).
func pgBinDir() (string, error) {
	cands := extraBinDirs()
	if path := os.Getenv("PATH"); path != "" {
		cands = append(cands, filepath.SplitList(path)...)
	}
	seen := map[string]bool{}
	for _, dir := range cands {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		ok := true
		for _, tool := range []string{"initdb", "pg_ctl", "postgres", "psql"} {
			fi, err := os.Stat(filepath.Join(dir, tool))
			if err != nil || fi.IsDir() || fi.Mode().Perm()&0o111 == 0 {
				ok = false
				break
			}
		}
		if ok {
			return dir, nil
		}
	}
	return "", fmt.Errorf("no directory with initdb/pg_ctl/postgres/psql on PATH or well-known locations")
}

// extraBinDirs lists well-known runtime locations beyond PATH. The console
// scrubs PATH for shaped servers, so the launcher cannot rely on the install
// environment surviving to run time.
func extraBinDirs() []string {
	dirs := []string{"/usr/local/bin", "/opt/homebrew/bin"}
	for _, v := range []string{"17", "16", "15", "14"} {
		dirs = append(dirs,
			"/usr/lib/postgresql/"+v+"/bin",
			"/usr/pgsql-"+v+"/bin",
			"/opt/homebrew/opt/postgresql@"+v+"/bin",
		)
	}
	return dirs
}

func lookPath(name string, extras []string) (string, error) {
	dirs := append([]string{}, extras...)
	if path := os.Getenv("PATH"); path != "" {
		dirs = append(dirs, filepath.SplitList(path)...)
	}
	seen := map[string]bool{}
	for _, dir := range dirs {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		full := filepath.Join(dir, name)
		if fi, err := os.Stat(full); err == nil && !fi.IsDir() && fi.Mode().Perm()&0o111 != 0 {
			return full, nil
		}
	}
	return "", fmt.Errorf("%s not found", name)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func fieldsVersion(s string) string {
	f := strings.Fields(s)
	if len(f) >= 3 {
		return strings.SplitN(f[2], ".", 2)[0]
	}
	return ""
}

func lastLines(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
