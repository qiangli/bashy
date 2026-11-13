// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Package main implements clip: an asynchronous clip library. A job is a
// small JSON file; queueing writes the file and returns at once, a worker
// command renders queued jobs one at a time, and finished clips land in a
// library directory the web page serves.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Job statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Job kinds.
const (
	KindTrim   = "trim"
	KindCast   = "cast"
	KindScreen = "screen"
)

// MaxSpanSeconds bounds one clip: a clip is short by definition.
const MaxSpanSeconds = 600

// Job is one queued clip request, stored as jobs/<id>.json under the data
// directory. All fields are plain data so any process (CLI, web page) can
// read the whole library by listing files.
type Job struct {
	Version  int     `json:"version"`
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Name     string  `json:"name,omitempty"`
	Source   string  `json:"source,omitempty"`
	Start    float64 `json:"start,omitempty"`
	End      float64 `json:"end,omitempty"`
	Seconds  float64 `json:"seconds,omitempty"`
	Format   string  `json:"format,omitempty"`  // cast: gif (default) or mp4
	Display  string  `json:"display,omitempty"` // screen: X11 display, e.g. :0.0
	Size     string  `json:"size,omitempty"`    // screen: WxH, e.g. 1280x720
	Status   string  `json:"status"`
	Error    string  `json:"error,omitempty"`
	Artifact string  `json:"artifact,omitempty"` // relative to the data dir
	Log      string  `json:"log,omitempty"`      // bounded worker output tail
	Created  string  `json:"created"`
	Updated  string  `json:"updated"`
}

// Span returns the clip's length in seconds, for display.
func (j *Job) Span() float64 {
	switch j.Kind {
	case KindScreen:
		return j.Seconds
	default:
		return j.End - j.Start
	}
}

// Store is the clip library on disk: DIR/jobs/*.json plus DIR/clips/*.
// Every method is file-based so the CLI and the web page share one store
// with no daemon between them.
type Store struct {
	dir string
}

// NewStore returns the store rooted at dir, creating its layout.
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("clip: no data directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(abs, "jobs"), 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(abs, "clips"), 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: abs}, nil
}

// Dir is the store's root directory.
func (s *Store) Dir() string { return s.dir }

// ClipsDir is where finished clips land.
func (s *Store) ClipsDir() string { return filepath.Join(s.dir, "clips") }

func (s *Store) jobPath(id string) string { return filepath.Join(s.dir, "jobs", id+".json") }

// ValidID accepts the ids this package mints: c plus hex, kept short so a
// person can type one.
func ValidID(id string) bool {
	if len(id) < 5 || len(id) > 32 || id[0] != 'c' {
		return false
	}
	_, err := hex.DecodeString(id[1:])
	return err == nil
}

func newID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "c" + hex.EncodeToString(b[:]), nil
}

// Save writes one job atomically: a temp file in the same directory,
// renamed over the target, so a reader never sees a half-written job.
func (s *Store) Save(j *Job) error {
	if j.ID == "" || !ValidID(j.ID) {
		return fmt.Errorf("clip: invalid job id %q", j.ID)
	}
	j.Updated = time.Now().UTC().Format(time.RFC3339Nano)
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Join(s.dir, "jobs"), j.ID+".tmp*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.jobPath(j.ID))
}

// Load reads one job by id.
func (s *Store) Load(id string) (*Job, error) {
	if !ValidID(id) {
		return nil, fmt.Errorf("clip: invalid job id %q", id)
	}
	raw, err := os.ReadFile(s.jobPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("clip: no job %s", id)
		}
		return nil, err
	}
	j := &Job{}
	if err := json.Unmarshal(raw, j); err != nil {
		return nil, fmt.Errorf("clip: saved job %s is invalid; preserve it for recovery", id)
	}
	if j.Version != 1 || j.ID != id {
		return nil, fmt.Errorf("clip: saved job %s is invalid; preserve it for recovery", id)
	}
	return j, nil
}

// List returns every job, newest first.
func (s *Store) List() ([]*Job, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "jobs"))
	if err != nil {
		return nil, err
	}
	var jobs []*Job
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		j, err := s.Load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			continue // an unreadable job never hides the rest of the library
		}
		jobs = append(jobs, j)
	}
	sort.Slice(jobs, func(a, b int) bool {
		if jobs[a].Created != jobs[b].Created {
			return jobs[a].Created > jobs[b].Created
		}
		return jobs[a].ID > jobs[b].ID
	})
	return jobs, nil
}

// Next returns the oldest queued job, or nil when none is waiting.
func (s *Store) Next() (*Job, error) {
	jobs, err := s.List()
	if err != nil {
		return nil, err
	}
	for i := len(jobs) - 1; i >= 0; i-- {
		if jobs[i].Status == StatusQueued {
			return jobs[i], nil
		}
	}
	return nil, nil
}

// Queue validates spec and records it as a queued job.
func (s *Store) Queue(spec *Job) (*Job, error) {
	if err := validate(spec); err != nil {
		return nil, err
	}
	id, err := newID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	j := *spec
	j.Version = 1
	j.ID = id
	j.Status = StatusQueued
	j.Created, j.Updated = now, now
	if err := s.Save(&j); err != nil {
		return nil, err
	}
	return &j, nil
}

var trimExts = map[string]bool{".mp4": true, ".m4v": true, ".webm": true, ".mov": true, ".mkv": true}

func validate(j *Job) error {
	if j.Name != "" {
		if len(j.Name) > 80 {
			return errors.New("clip: name is longer than 80 characters")
		}
		for _, r := range j.Name {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ' ' || r == '-' || r == '_' || r == '.') {
				return errors.New("clip: name may use letters, digits, space, - _ . only")
			}
		}
	}
	switch j.Kind {
	case KindTrim, KindCast:
		if j.Source == "" {
			return fmt.Errorf("clip: %s needs --from PATH", j.Kind)
		}
		if !filepath.IsAbs(j.Source) {
			return errors.New("clip: --from must be an absolute path")
		}
		info, err := os.Stat(j.Source)
		if err != nil {
			return fmt.Errorf("clip: source: %v", err)
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return errors.New("clip: source must be a regular file")
		}
		ext := strings.ToLower(filepath.Ext(j.Source))
		if j.Kind == KindTrim && !trimExts[ext] {
			return fmt.Errorf("clip: trim needs a video file (.mp4 .m4v .webm .mov .mkv), got %q", ext)
		}
		if j.Kind == KindCast && ext != ".cast" {
			return errors.New("clip: cast needs an asciicast v2 file (.cast)")
		}
		if j.Start < 0 || j.End <= j.Start {
			return errors.New("clip: --start must be >= 0 and --end greater than --start")
		}
		if j.End-j.Start > MaxSpanSeconds {
			return fmt.Errorf("clip: a clip is at most %d seconds", MaxSpanSeconds)
		}
		if j.Kind == KindCast && j.Format == "" {
			j.Format = "gif"
		}
		if j.Format != "" && j.Format != "gif" && j.Format != "mp4" {
			return errors.New("clip: --format is gif or mp4")
		}
	case KindScreen:
		if j.Seconds < 1 || j.Seconds > MaxSpanSeconds {
			return fmt.Errorf("clip: --seconds is 1..%d", MaxSpanSeconds)
		}
		if j.Size != "" && !isSize(j.Size) {
			return errors.New("clip: --size is WIDTHxHEIGHT, e.g. 1280x720")
		}
		if j.Display != "" && (len(j.Display) > 32 || strings.ContainsAny(j.Display, " \t\n")) {
			return errors.New("clip: --display is an X11 display like :0.0")
		}
	default:
		return fmt.Errorf("clip: kind is trim, cast or screen, got %q", j.Kind)
	}
	return nil
}

func isSize(s string) bool {
	x, y, ok := strings.Cut(s, "x")
	if !ok {
		return false
	}
	n := func(v string) bool {
		if v == "" || len(v) > 5 {
			return false
		}
		for _, r := range v {
			if r < '0' || r > '9' {
				return false
			}
		}
		return true
	}
	return n(x) && n(y)
}
