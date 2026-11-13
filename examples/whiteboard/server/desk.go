package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type DeskHandler struct {
	mu      sync.Mutex
	baseDir string
	prefix  string
}

type BoardRecord struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Revision int      `json:"revision"`
	Locked   bool     `json:"locked"`
	OwnerKey string   `json:"key,omitempty"`
	Notes    []string `json:"notes,omitempty"`
}

func NewDeskHandler(dataDir, prefix string) *DeskHandler {
	return &DeskHandler{
		baseDir: dataDir,
		prefix:  prefix,
	}
}

func (d *DeskHandler) boardFile(id string) string {
	return filepath.Join(d.baseDir, "boards", id+".json")
}

func (d *DeskHandler) loadBoard(id string) (*BoardRecord, error) {
	data, err := os.ReadFile(d.boardFile(id))
	if err != nil {
		return nil, err
	}
	var b BoardRecord
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

func (d *DeskHandler) saveBoard(b *BoardRecord) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.boardFile(b.ID), data, 0600)
}

func (d *DeskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasPrefix(path, "/whiteboard") {
		path = strings.TrimPrefix(path, "/whiteboard")
	}

	switch path {
	case "/healthz":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	case "/api/board":
		d.handleListBoards(w, r)
	case "/api/state":
		d.handleState(w, r)
	case "/api/action":
		d.handleAction(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (d *DeskHandler) handleListBoards(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	matches, _ := filepath.Glob(filepath.Join(d.baseDir, "boards", "*.json"))
	type item struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Revision int    `json:"revision"`
		Locked   bool   `json:"locked"`
		Status   string `json:"status"`
		URL      string `json:"url"`
	}
	list := make([]item, 0, len(matches))
	for _, m := range matches {
		id := strings.TrimSuffix(filepath.Base(m), ".json")
		b, err := d.loadBoard(id)
		if err == nil {
			st := "pending"
			if b.Locked {
				st = "closed"
			}
			list = append(list, item{
				ID:       b.ID,
				Name:     b.Name,
				Revision: b.Revision,
				Locked:   b.Locked,
				Status:   st,
				URL:      d.prefix + "/#room=" + b.ID,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"running": true,
		"boards":  list,
		"data":    filepath.Join(d.baseDir, "boards"),
	})
}

func (d *DeskHandler) handleState(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		id = r.URL.Query().Get("board")
	}
	if id == "" {
		id = "main"
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	b, err := d.loadBoard(id)
	if err != nil {
		// Auto-initialize default board
		b = &BoardRecord{ID: id, Name: id, Revision: 1, Locked: false}
		_ = d.saveBoard(b)
	}

	st := "pending"
	if b.Locked {
		st = "closed"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":       b.ID,
		"name":     b.Name,
		"revision": b.Revision,
		"locked":   b.Locked,
		"status":   st,
		"notes":    b.Notes,
		"url":      d.prefix + "/#room=" + b.ID,
	})
}

type actionRequest struct {
	Action string `json:"action"`
	ID     string `json:"id"`
	Board  string `json:"board"`
	Body   string `json:"body"`
	Key    string `json:"key"`
	Locked *bool  `json:"locked"`
}

func (d *DeskHandler) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req actionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	id := req.ID
	if id == "" {
		id = req.Board
	}
	if id == "" && req.Action != "add" && req.Action != "list" {
		id = "main"
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	switch req.Action {
	case "add":
		if id == "" {
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			id = hex.EncodeToString(buf)
		}
		keyBuf := make([]byte, 16)
		_, _ = rand.Read(keyBuf)
		key := hex.EncodeToString(keyBuf)

		b := &BoardRecord{
			ID:       id,
			Name:     id,
			Revision: 1,
			Locked:   false,
			OwnerKey: key,
		}
		if req.Body != "" {
			b.Notes = append(b.Notes, req.Body)
		}
		_ = d.saveBoard(b)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     b.ID,
			"url":    d.prefix + "/#room=" + b.ID,
			"status": "pending",
			"key":    key,
		})

	case "open", "show":
		b, err := d.loadBoard(id)
		if err != nil {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}
		st := "pending"
		if b.Locked {
			st = "closed"
		}
		res := map[string]any{
			"id":       b.ID,
			"url":      d.prefix + "/#room=" + b.ID,
			"status":   st,
			"revision": b.Revision,
			"locked":   b.Locked,
		}
		if req.Action == "show" {
			res["notes"] = b.Notes
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)

	case "set":
		b, err := d.loadBoard(id)
		if err != nil {
			b = &BoardRecord{ID: id, Name: id, Revision: 0}
		}
		if req.Body != "" {
			b.Notes = append(b.Notes, req.Body)
		}
		b.Revision++
		_ = d.saveBoard(b)

		st := "pending"
		if b.Locked {
			st = "closed"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       b.ID,
			"url":      d.prefix + "/#room=" + b.ID,
			"status":   st,
			"revision": b.Revision,
		})

	case "close":
		b, err := d.loadBoard(id)
		if err != nil {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}
		b.Locked = true
		b.Revision++
		_ = d.saveBoard(b)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       b.ID,
			"url":      d.prefix + "/#room=" + b.ID,
			"status":   "closed",
			"revision": b.Revision,
		})

	case "list":
		d.handleListBoards(w, r)

	case "share":
		b, err := d.loadBoard(id)
		if err != nil {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":  b.ID,
			"url": d.prefix + "/#room=" + b.ID,
		})

	case "remove":
		if _, err := d.loadBoard(id); err != nil {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}
		if err := os.Remove(d.boardFile(id)); err != nil {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     id,
			"status": "removed",
		})

	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}
