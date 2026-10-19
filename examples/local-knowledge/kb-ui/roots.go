// Copyright (c) 2026 Qiang Li. All rights reserved.
// Ported from kg-ui under explicit operator authorization for Bashy.
// See ATTRIBUTION.md and LICENSE for licensing information.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RootConfig represents an explicitly selected root for indexing and file preview.
type RootConfig struct {
	Path      string    `json:"path"`
	Resolved  string    `json:"resolved"`
	Status    string    `json:"status"` // "active", "paused", "completed", "error"
	Includes  []string  `json:"includes"`
	Excludes  []string  `json:"excludes"`
	Progress  float64   `json:"progress"`
	FileCount int       `json:"file_count"`
	ByteCount int64     `json:"byte_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Error     string    `json:"error,omitempty"`
}

// ResolvePath resolves relative paths and expands `~` to the user's home directory.
func ResolvePath(rawPath string) (string, error) {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return "", ErrInvalidPath
	}

	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		p = home
	} else if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~/"))
	}

	abs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	return abs, nil
}

// RootsManager manages the set of eligible roots. Thread-safe.
type RootsManager struct {
	mu    sync.RWMutex
	roots map[string]*RootConfig // keyed by resolved absolute path
}

// NewRootsManager creates a RootsManager initialized with the given paths.
func NewRootsManager(initialPaths []string) *RootsManager {
	rm := &RootsManager{
		roots: make(map[string]*RootConfig),
	}
	for _, p := range initialPaths {
		if strings.TrimSpace(p) != "" {
			_, _ = rm.Add(p, nil, nil)
		}
	}
	return rm
}

// Add registers a new root. Path with `~` is automatically resolved.
func (rm *RootsManager) Add(rawPath string, includes, excludes []string) (*RootConfig, error) {
	resolved, err := ResolvePath(rawPath)
	if err != nil {
		return nil, err
	}

	// Verify path exists on disk
	fi, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("root path does not exist: %w", err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("root path is not a directory: %s", resolved)
	}

	if len(includes) == 0 {
		includes = []string{"*"}
	}
	if len(excludes) == 0 {
		excludes = []string{".git", "node_modules", ".tmp", "dist", "build"}
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	cfg := &RootConfig{
		Path:      rawPath,
		Resolved:  resolved,
		Status:    "active",
		Includes:  includes,
		Excludes:  excludes,
		Progress:  0.0,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	rm.roots[resolved] = cfg
	return cfg, nil
}

// Remove unregisters a root by its raw or resolved path.
func (rm *RootsManager) Remove(rawOrResolved string) bool {
	resolved, err := ResolvePath(rawOrResolved)
	if err != nil {
		resolved = filepath.Clean(rawOrResolved)
	}
	evalResolved, _ := filepath.EvalSymlinks(resolved)
	cleanRaw := filepath.Clean(rawOrResolved)

	rm.mu.Lock()
	defer rm.mu.Unlock()

	for k, v := range rm.roots {
		kEval, _ := filepath.EvalSymlinks(k)
		if k == resolved || k == cleanRaw || (evalResolved != "" && kEval == evalResolved) ||
			v.Path == rawOrResolved || v.Path == cleanRaw || (evalResolved != "" && v.Resolved == evalResolved) {
			delete(rm.roots, k)
			return true
		}
	}
	return false
}

// Pause pauses indexing for a specific root.
func (rm *RootsManager) Pause(rawOrResolved string) error {
	resolved, err := ResolvePath(rawOrResolved)
	if err != nil {
		resolved = filepath.Clean(rawOrResolved)
	}
	evalResolved, _ := filepath.EvalSymlinks(resolved)
	cleanRaw := filepath.Clean(rawOrResolved)

	rm.mu.Lock()
	defer rm.mu.Unlock()

	for k, cfg := range rm.roots {
		kEval, _ := filepath.EvalSymlinks(k)
		if k == resolved || k == cleanRaw || (evalResolved != "" && kEval == evalResolved) ||
			cfg.Path == rawOrResolved || cfg.Path == cleanRaw {
			cfg.Status = "paused"
			cfg.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return fmt.Errorf("root not found: %s", rawOrResolved)
}

// Resume resumes indexing for a specific root.
func (rm *RootsManager) Resume(rawOrResolved string) error {
	resolved, err := ResolvePath(rawOrResolved)
	if err != nil {
		resolved = filepath.Clean(rawOrResolved)
	}
	evalResolved, _ := filepath.EvalSymlinks(resolved)
	cleanRaw := filepath.Clean(rawOrResolved)

	rm.mu.Lock()
	defer rm.mu.Unlock()

	for k, cfg := range rm.roots {
		kEval, _ := filepath.EvalSymlinks(k)
		if k == resolved || k == cleanRaw || (evalResolved != "" && kEval == evalResolved) ||
			cfg.Path == rawOrResolved || cfg.Path == cleanRaw {
			cfg.Status = "active"
			cfg.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return fmt.Errorf("root not found: %s", rawOrResolved)
}

// List returns a slice of all registered root configs.
func (rm *RootsManager) List() []RootConfig {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	out := make([]RootConfig, 0, len(rm.roots))
	for _, v := range rm.roots {
		out = append(out, *v)
	}
	return out
}

// GetEligibleRoots returns a slice of all resolved paths for file preview containment checks.
func (rm *RootsManager) GetEligibleRoots() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	out := make([]string, 0, len(rm.roots))
	for k := range rm.roots {
		out = append(out, k)
	}
	return out
}

// UpdateCounts updates file count, bytes, and progress for a root.
func (rm *RootsManager) UpdateCounts(resolved string, fileCount int, byteCount int64, progress float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if cfg, ok := rm.roots[resolved]; ok {
		cfg.FileCount = fileCount
		cfg.ByteCount = byteCount
		cfg.Progress = progress
		cfg.UpdatedAt = time.Now().UTC()
	}
}

// Replace refreshes the presentation cache from authoritative shared index state.
func (rm *RootsManager) Replace(roots []RootConfig) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.roots = map[string]*RootConfig{}
	for _, r := range roots {
		copy := r
		rm.roots[r.Resolved] = &copy
	}
}
