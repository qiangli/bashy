// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Package cmdcatalog holds the embedded per-verb catalog entries that bashy
// dispatches through generated mains and yoke/pkg/shape/slot side slots.
//
// Each command is its own YAML file under entries/<name>.yaml so parallel P4
// workers converting disjoint verb lists add entries without merge conflicts.
package cmdcatalog

import (
	"embed"
	"os"
	"strings"
	"sync"

	"github.com/qiangli/bashy/internal/cmdgen"
)

//go:embed all:entries
var embeddedFS embed.FS

var (
	embeddedOnce    sync.Once
	embeddedEntries []cmdgen.Entry
	embeddedByName  map[string]cmdgen.Entry
	embeddedErr     error
)

func loadEmbedded() {
	embeddedOnce.Do(func() {
		embeddedEntries, embeddedErr = cmdgen.LoadCatalogFS(embeddedFS, "entries")
		embeddedByName = make(map[string]cmdgen.Entry, len(embeddedEntries))
		for _, e := range embeddedEntries {
			for _, n := range e.Names() {
				embeddedByName[n] = e
			}
		}
	})
}

// Entries returns all embedded catalog entries (plus any entries overlaid from
// $BASHY_CMD_CATALOG_DIR when set).
func Entries() []cmdgen.Entry {
	loadEmbedded()
	if overlay := strings.TrimSpace(os.Getenv("BASHY_CMD_CATALOG_DIR")); overlay != "" {
		if extra, err := cmdgen.LoadCatalog(overlay); err == nil {
			byName := make(map[string]cmdgen.Entry, len(embeddedEntries)+len(extra))
			for _, e := range embeddedEntries {
				byName[e.Name] = e
			}
			for _, e := range extra {
				byName[e.Name] = e
			}
			out := make([]cmdgen.Entry, 0, len(byName))
			for _, e := range byName {
				out = append(out, e)
			}
			return out
		}
	}
	return append([]cmdgen.Entry(nil), embeddedEntries...)
}

// Lookup resolves a command by canonical name or alias. $BASHY_CMD_CATALOG_DIR,
// when set, overlays the embedded catalog (used by tests and local iteration).
func Lookup(name string) (cmdgen.Entry, bool) {
	if name == "" {
		return cmdgen.Entry{}, false
	}
	if overlay := strings.TrimSpace(os.Getenv("BASHY_CMD_CATALOG_DIR")); overlay != "" {
		if extra, err := cmdgen.LoadCatalog(overlay); err == nil {
			for _, e := range extra {
				for _, n := range e.Names() {
					if n == name {
						return e, true
					}
				}
			}
		}
	}
	loadEmbedded()
	e, ok := embeddedByName[name]
	return e, ok
}
