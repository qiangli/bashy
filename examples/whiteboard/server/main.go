package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed index.html
var page []byte

// Logical canvas all browsers share; the page scales it to fit.
const (
	canvasW    = 1600.0
	canvasH    = 1000.0
	maxPoints  = 300
	maxStrokes = 2000
	maxNotes   = 1000
)

var boardNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Pt struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Stroke struct {
	ID     string  `json:"id"`
	Points []Pt    `json:"points"`
	Color  string  `json:"color"`
	Width  float64 `json:"width"`
	Owner  string  `json:"owner"`
}
type Note struct {
	ID    string  `json:"id"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Text  string  `json:"text"`
	Color string  `json:"color"`
	Owner string  `json:"owner"`
}
type Board struct {
	Version     int      `json:"version"`
	Name        string   `json:"name"`
	Facilitator string   `json:"facilitator"`
	Locked      bool     `json:"locked"`
	Strokes     []Stroke `json:"strokes"`
	Notes       []Note   `json:"notes"`
	Revision    int      `json:"revision"`
}
type app struct {
	mu     sync.Mutex
	dir    string
	prefix string
	boards map[string]*Board
}

func token() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func validToken(s string) bool {
	if len(s) != 48 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func boardPath(dir, name string) string {
	return filepath.Join(dir, "boards", name+".json")
}
func saveBoard(path string, b Board) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".board-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

var errBoardNotFound = errors.New("board not found")
var errBoardExists = errors.New("board already exists")

// loadLocked returns the named board without creating it. Callers must hold
// a.mu. Only the explicit create action mints a new board (see createLocked);
// reads of unknown names return errBoardNotFound so guests cannot reserve
// names or fill disk. Saved boards on disk are still loaded on first read.
func (a *app) loadLocked(name string) (*Board, error) {
	if !boardNameRe.MatchString(name) {
		return nil, errors.New("use 1-64 letters, digits, - or _ for a board name")
	}
	if b, ok := a.boards[name]; ok {
		return b, nil
	}
	path := boardPath(a.dir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errBoardNotFound
		}
		return nil, err
	}
	b := &Board{}
	if err = json.Unmarshal(raw, b); err != nil {
		return nil, fmt.Errorf("saved board %q is invalid; preserve it for recovery: %w", name, err)
	}
	if b.Version != 1 || !validToken(b.Facilitator) || b.Name != name {
		return nil, fmt.Errorf("unsupported or invalid saved board %q", name)
	}
	if b.Strokes == nil {
		b.Strokes = []Stroke{}
	}
	if b.Notes == nil {
		b.Notes = []Note{}
	}
	a.boards[name] = b
	return b, nil
}

// createLocked mints and persists a new board. Callers must hold a.mu and
// have verified the name is valid and unused. The minted facilitator
// capability is revealed only in the create-action response, never in state
// or exports.
func (a *app) createLocked(name string) (*Board, error) {
	if !boardNameRe.MatchString(name) {
		return nil, errors.New("use 1-64 letters, digits, - or _ for a board name")
	}
	if _, ok := a.boards[name]; ok {
		return nil, errBoardExists
	}
	if _, err := os.Stat(boardPath(a.dir, name)); err == nil {
		return nil, errBoardExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	b := &Board{Version: 1, Name: name, Facilitator: token(), Strokes: []Stroke{}, Notes: []Note{}}
	if err := saveBoard(boardPath(a.dir, name), *b); err != nil {
		return nil, err
	}
	a.boards[name] = b
	return b, nil
}
func boardNotFound(w http.ResponseWriter) {
	fail(w, 404, "Board not found. Ask the facilitator to create it.")
}
func (a *app) facilitator(r *http.Request, b *Board) bool {
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Facilitator-Key")), []byte(b.Facilitator)) == 1
}
func participant(w http.ResponseWriter, r *http.Request) string {
	if c, e := r.Cookie("whiteboard-participant"); e == nil && validToken(c.Value) {
		return c.Value
	}
	id := token()
	http.SetCookie(w, &http.Cookie{Name: "whiteboard-participant", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 86400 * 30})
	return id
}
func respond(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
func validPt(p Pt) bool {
	return !math.IsNaN(p.X) && !math.IsNaN(p.Y) && !math.IsInf(p.X, 0) && !math.IsInf(p.Y, 0) &&
		p.X >= 0 && p.X <= canvasW && p.Y >= 0 && p.Y <= canvasH
}
func (a *app) stripPrefix(path string) (string, bool) {
	if a.prefix == "" {
		return path, true
	}
	if path == a.prefix {
		return "/", true
	}
	if strings.HasPrefix(path, a.prefix+"/") {
		return strings.TrimPrefix(path, a.prefix), true
	}
	return "", false
}
func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if r.Method != "GET" && r.Method != "POST" {
		fail(w, 405, "Method not supported")
		return
	}
	if r.Method == "POST" {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host {
				fail(w, 403, "Use the board's own page")
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			fail(w, 403, "Cross-site request refused")
			return
		}
	}
	path, ok := a.stripPrefix(r.URL.Path)
	if !ok {
		fail(w, 404, "Not found")
		return
	}
	if path == "/" && r.Method == "GET" {
		participant(w, r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
		return
	}
	if path == "/healthz" && r.Method == "GET" {
		respond(w, map[string]bool{"ok": true})
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	id := participant(w, r)
	if path == "/api/state" && r.Method == "GET" {
		name := r.URL.Query().Get("board")
		if name == "" {
			name = "main"
		}
		b, err := a.loadLocked(name)
		if err != nil {
			if errors.Is(err, errBoardNotFound) {
				boardNotFound(w)
			} else {
				fail(w, 400, err.Error())
			}
			return
		}
		respond(w, a.view(id, b, a.facilitator(r, b)))
		return
	}
	if path == "/api/export" && r.Method == "GET" {
		name := r.URL.Query().Get("board")
		if name == "" {
			name = "main"
		}
		b, err := a.loadLocked(name)
		if err != nil {
			if errors.Is(err, errBoardNotFound) {
				boardNotFound(w)
			} else {
				fail(w, 400, err.Error())
			}
			return
		}
		if r.URL.Query().Get("format") == "png" {
			img := renderPNG(b)
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Disposition", "attachment; filename=whiteboard-"+name+".png")
			w.Write(img)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=whiteboard-"+name+".json")
		respond(w, exportView(b))
		return
	}
	if path != "/api/action" || r.Method != "POST" {
		fail(w, 404, "Not found")
		return
	}
	var req struct {
		Action string  `json:"action"`
		Board  string  `json:"board"`
		ID     string  `json:"id"`
		Points []Pt    `json:"points"`
		Color  string  `json:"color"`
		Width  float64 `json:"width"`
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Text   string  `json:"text"`
		Locked bool    `json:"locked"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		fail(w, 400, "Invalid request")
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		fail(w, 400, "Invalid request")
		return
	}
	if req.Board == "" {
		req.Board = "main"
	}
	if !boardNameRe.MatchString(req.Board) {
		fail(w, 400, "Use 1-64 letters, digits, - or _ for a board name")
		return
	}
	if req.Action == "create" {
		b, err := a.createLocked(req.Board)
		if err != nil {
			if errors.Is(err, errBoardExists) {
				fail(w, 409, "A board with that name already exists")
			} else if !boardNameRe.MatchString(req.Board) {
				fail(w, 400, err.Error())
			} else {
				log.Printf("create failed: %v", err)
				fail(w, 500, "Could not save. Your change was not accepted; keep your work and ask the facilitator.")
			}
			return
		}
		v := a.view(id, b, true)
		v["facilitatorKey"] = b.Facilitator
		respond(w, v)
		return
	}
	b, err := a.loadLocked(req.Board)
	if err != nil {
		if errors.Is(err, errBoardNotFound) {
			boardNotFound(w)
		} else {
			fail(w, 400, err.Error())
		}
		return
	}
	isFac := a.facilitator(r, b)
	// Work on a copy: a failed disk save must not acknowledge or retain a mutation.
	raw, _ := json.Marshal(b)
	var next Board
	json.Unmarshal(raw, &next)
	lockedFor := func() bool { return next.Locked && !isFac }
	findStroke, findNote := -1, -1
	for i, s := range next.Strokes {
		if s.ID == req.ID {
			findStroke = i
			break
		}
	}
	for i, n := range next.Notes {
		if n.ID == req.ID {
			findNote = i
			break
		}
	}
	switch req.Action {
	case "stroke":
		if lockedFor() {
			fail(w, 403, "The board is locked; ask the facilitator to unlock it")
			return
		}
		if len(req.Points) < 2 || len(req.Points) > maxPoints {
			fail(w, 400, "A stroke needs 2-300 points inside the canvas")
			return
		}
		for _, p := range req.Points {
			if !validPt(p) {
				fail(w, 400, "Keep strokes inside the 1600x1000 canvas")
				return
			}
		}
		if !colorRe.MatchString(req.Color) {
			fail(w, 400, "Pick a pen color")
			return
		}
		if req.Width < 1 || req.Width > 30 {
			fail(w, 400, "Pen width must be 1-30")
			return
		}
		if len(next.Strokes) >= maxStrokes {
			fail(w, 409, "This board has reached its 2,000-stroke limit")
			return
		}
		next.Strokes = append(next.Strokes, Stroke{ID: token(), Points: req.Points, Color: req.Color, Width: req.Width, Owner: id})
	case "note":
		if lockedFor() {
			fail(w, 403, "The board is locked; ask the facilitator to unlock it")
			return
		}
		req.Text = strings.TrimSpace(req.Text)
		if req.Text == "" || len([]rune(req.Text)) > 500 || !validPt(Pt{X: req.X, Y: req.Y}) {
			fail(w, 400, "Place a 1-500 character note inside the canvas")
			return
		}
		if req.Color != "" && !colorRe.MatchString(req.Color) {
			fail(w, 400, "Pick a note color")
			return
		}
		if req.Color == "" {
			req.Color = "#fff3bf"
		}
		if len(next.Notes) >= maxNotes {
			fail(w, 409, "This board has reached its 1,000-note limit")
			return
		}
		next.Notes = append(next.Notes, Note{ID: token(), X: req.X, Y: req.Y, Text: req.Text, Color: req.Color, Owner: id})
	case "move-note":
		if findNote < 0 {
			fail(w, 404, "Note not found")
			return
		}
		if next.Notes[findNote].Owner != id && !isFac {
			fail(w, 403, "You can move only your own notes")
			return
		}
		if lockedFor() {
			fail(w, 403, "The board is locked; ask the facilitator to unlock it")
			return
		}
		if !validPt(Pt{X: req.X, Y: req.Y}) {
			fail(w, 400, "Keep notes inside the 1600x1000 canvas")
			return
		}
		next.Notes[findNote].X, next.Notes[findNote].Y = req.X, req.Y
	case "edit-note":
		if findNote < 0 {
			fail(w, 404, "Note not found")
			return
		}
		if next.Notes[findNote].Owner != id && !isFac {
			fail(w, 403, "You can edit only your own notes")
			return
		}
		if lockedFor() {
			fail(w, 403, "The board is locked; ask the facilitator to unlock it")
			return
		}
		req.Text = strings.TrimSpace(req.Text)
		if req.Text == "" || len([]rune(req.Text)) > 500 {
			fail(w, 400, "Keep notes to 1-500 characters")
			return
		}
		next.Notes[findNote].Text = req.Text
	case "delete-note":
		if findNote < 0 {
			fail(w, 404, "Note not found")
			return
		}
		if next.Notes[findNote].Owner != id && !isFac {
			fail(w, 403, "You can delete only your own notes")
			return
		}
		if lockedFor() {
			fail(w, 403, "The board is locked; ask the facilitator to unlock it")
			return
		}
		next.Notes = append(next.Notes[:findNote], next.Notes[findNote+1:]...)
	case "delete-stroke":
		if findStroke < 0 {
			fail(w, 404, "Stroke not found")
			return
		}
		if next.Strokes[findStroke].Owner != id && !isFac {
			fail(w, 403, "You can delete only your own strokes")
			return
		}
		if lockedFor() {
			fail(w, 403, "The board is locked; ask the facilitator to unlock it")
			return
		}
		next.Strokes = append(next.Strokes[:findStroke], next.Strokes[findStroke+1:]...)
	case "clear":
		if !isFac {
			fail(w, 403, "Only the facilitator can clear the board")
			return
		}
		next.Strokes = []Stroke{}
		next.Notes = []Note{}
	case "lock":
		if !isFac {
			fail(w, 403, "Only the facilitator can lock the board")
			return
		}
		next.Locked = req.Locked
	default:
		fail(w, 400, "Unknown action")
		return
	}
	next.Revision++
	if err := saveBoard(boardPath(a.dir, next.Name), next); err != nil {
		log.Printf("save failed: %v", err)
		fail(w, 500, "Could not save. Your change was not accepted; keep your work and ask the facilitator.")
		return
	}
	*a.boards[next.Name] = next
	respond(w, a.view(id, &next, isFac))
}
func (a *app) view(id string, b *Board, fac bool) map[string]any {
	strokes := []map[string]any{}
	for _, s := range b.Strokes {
		pts := make([]map[string]float64, 0, len(s.Points))
		for _, p := range s.Points {
			pts = append(pts, map[string]float64{"x": p.X, "y": p.Y})
		}
		strokes = append(strokes, map[string]any{"id": s.ID, "points": pts, "color": s.Color, "width": s.Width, "mine": s.Owner == id})
	}
	notes := []map[string]any{}
	for _, n := range b.Notes {
		notes = append(notes, map[string]any{"id": n.ID, "x": n.X, "y": n.Y, "text": n.Text, "color": n.Color, "mine": n.Owner == id})
	}
	return map[string]any{"board": b.Name, "revision": b.Revision, "locked": b.Locked,
		"strokes": strokes, "notes": notes, "facilitator": fac,
		"canvas": map[string]float64{"w": canvasW, "h": canvasH}}
}
func exportView(b *Board) any {
	strokes := []map[string]any{}
	for _, s := range b.Strokes {
		pts := make([]map[string]float64, 0, len(s.Points))
		for _, p := range s.Points {
			pts = append(pts, map[string]float64{"x": p.X, "y": p.Y})
		}
		strokes = append(strokes, map[string]any{"points": pts, "color": s.Color, "width": s.Width})
	}
	notes := []map[string]any{}
	for _, n := range b.Notes {
		notes = append(notes, map[string]any{"x": n.X, "y": n.Y, "text": n.Text, "color": n.Color})
	}
	return map[string]any{"board": b.Name, "revision": b.Revision, "locked": b.Locked,
		"canvas":  map[string]float64{"w": canvasW, "h": canvasH},
		"strokes": strokes, "notes": notes}
}
func hexColor(s string) color.RGBA {
	var r, g, b uint8
	fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
func dot(img *image.RGBA, cx, cy int, rad int, c color.Color) {
	b := img.Bounds()
	for dy := -rad; dy <= rad; dy++ {
		for dx := -rad; dx <= rad; dx++ {
			if dx*dx+dy*dy <= rad*rad {
				p := image.Point{X: cx + dx, Y: cy + dy}
				if p.In(b) {
					img.Set(p.X, p.Y, c)
				}
			}
		}
	}
}
func line(img *image.RGBA, x0, y0, x1, y1 float64, c color.Color, w float64) {
	steps := int(math.Hypot(x1-x0, y1-y0)) + 1
	rad := int(w / 2)
	if rad < 1 {
		rad = 1
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		dot(img, int(x0+(x1-x0)*t), int(y0+(y1-y0)*t), rad, c)
	}
}
func rect(img *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	b := img.Bounds()
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if (image.Point{X: x, Y: y}).In(b) {
				img.Set(x, y, c)
			}
		}
	}
}
func renderPNG(b *Board) []byte {
	const scale = 1
	img := image.NewRGBA(image.Rect(0, 0, int(canvasW)*scale, int(canvasH)*scale))
	white := color.RGBA{255, 255, 255, 255}
	rect(img, 0, 0, int(canvasW)-1, int(canvasH)-1, white)
	grid := color.RGBA{233, 238, 233, 255}
	for x := 0; x < int(canvasW); x += 100 {
		line(img, float64(x), 0, float64(x), canvasH, grid, 1)
	}
	for y := 0; y < int(canvasH); y += 100 {
		line(img, 0, float64(y), canvasW, float64(y), grid, 1)
	}
	for _, s := range b.Strokes {
		c := hexColor(s.Color)
		for i := 1; i < len(s.Points); i++ {
			line(img, s.Points[i-1].X, s.Points[i-1].Y, s.Points[i].X, s.Points[i].Y, c, s.Width)
		}
	}
	for _, n := range b.Notes {
		x0, y0 := int(n.X), int(n.Y)
		x1, y1 := x0+220, y0+120
		if x1 >= int(canvasW) {
			x1 = int(canvasW) - 1
		}
		if y1 >= int(canvasH) {
			y1 = int(canvasH) - 1
		}
		rect(img, x0, y0, x1, y1, hexColor(n.Color))
		border := color.RGBA{120, 110, 60, 255}
		line(img, float64(x0), float64(y0), float64(x1), float64(y0), border, 2)
		line(img, float64(x0), float64(y1), float64(x1), float64(y1), border, 2)
		line(img, float64(x0), float64(y0), float64(x0), float64(y1), border, 2)
		line(img, float64(x1), float64(y0), float64(x1), float64(y1), border, 2)
		ink := color.RGBA{60, 60, 60, 255}
		rows := 1 + len(n.Text)/28
		if rows > 4 {
			rows = 4
		}
		for r := 0; r < rows; r++ {
			line(img, float64(x0+12), float64(y0+28+r*22), float64(x1-12), float64(y0+28+r*22), ink, 2)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
func main() {
	data := flag.String("data", "", "persistent data directory")
	addr := flag.String("addr", "0.0.0.0:18093", "listen address")
	prefix := flag.String("prefix", "", "URL prefix when served behind a sub-path (e.g. /whiteboard)")
	flag.Parse()
	if *data == "" {
		log.Fatal("-data is required")
	}
	*prefix = strings.TrimSuffix(*prefix, "/")
	if *prefix != "" && !strings.HasPrefix(*prefix, "/") {
		log.Fatal("-prefix must start with /")
	}
	if err := os.MkdirAll(filepath.Join(*data, "boards"), 0700); err != nil {
		log.Fatal(err)
	}
	a := &app{dir: *data, prefix: *prefix, boards: map[string]*Board{}}
	a.mu.Lock()
	mainBoard, err := a.loadLocked("main")
	if errors.Is(err, errBoardNotFound) {
		mainBoard, err = a.createLocked("main")
	}
	a.mu.Unlock()
	if err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("Cannot listen on %s: %v. Set WHITEBOARD_ADDR to another port.", *addr, err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	host, _, _ := net.SplitHostPort(*addr)
	hosts := []string{"127.0.0.1"}
	if host != "" && host != "0.0.0.0" && host != "::" {
		hosts = []string{host}
	} else {
		addrs, _ := net.InterfaceAddrs()
		for _, n := range addrs {
			ip, _, e := net.ParseCIDR(n.String())
			if e == nil && ip.To4() != nil && ip.IsPrivate() && !ip.IsLoopback() {
				hosts = append(hosts, ip.String())
			}
		}
	}
	sort.Strings(hosts)
	base := func(h string) string { return "http://" + net.JoinHostPort(h, port) + *prefix + "/" }
	fmt.Println("\nShared whiteboard — keep this terminal running")
	for _, h := range hosts {
		fmt.Printf("Participant: %s?board=main\n", base(h))
	}
	facHost := "127.0.0.1"
	for _, h := range hosts {
		if h != "127.0.0.1" {
			facHost = h
			break
		}
	}
	fmt.Printf("Facilitator (keep private): %s?board=main#facilitator=%s\n", base(facHost), mainBoard.Facilitator)
	fmt.Printf("Saved boards: %s\nCtrl-C stops the server; restart resumes these boards.\n\n", filepath.Join(*data, "boards"))
	srv := &http.Server{Handler: a, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()
	if err = srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
