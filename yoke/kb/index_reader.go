// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

// kb's own copy of the indexed-file recall lane (duplicated from
// internal/agentos on purpose; see codering.go).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qiangli/yoke/pkg/indexer"
	"github.com/qiangli/yoke/pkg/kbmemory"
	"github.com/qiangli/yoke/pkg/recall"
)

// indexFileReader is a generated-source lane, not a curated kb store.
// Scores stay within this lane, preserving recall's per-ring ranking rule.
type indexFileReader struct{}

func (indexFileReader) Ring() string    { return "index" }
func (indexFileReader) Forms() []string { return []string{"file"} }
func (indexFileReader) Recall(q recall.Query) ([]recall.Hit, error) {
	home := os.Getenv("BASHY_HOME")
	if home == "" {
		h, e := os.UserHomeDir()
		if e != nil {
			return nil, e
		}
		home = filepath.Join(h, ".bashy")
	}
	if _, e := os.Stat(filepath.Join(home, "index", "state.json")); errors.Is(e, os.ErrNotExist) {
		return nil, nil
	} else if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	k := q.K
	if k <= 0 {
		k = 3
	}
	env := indexer.New(home).Query(ctx, q.Text, "auto", k)
	if !env.OK {
		return nil, fmt.Errorf("indexed recall: %s", env.Error.Message)
	}
	res := env.Result.(kbmemory.RetrieveResult)
	out := make([]recall.Hit, 0, len(res.Hits))
	for _, h := range res.Hits {
		body := h.Record.Body
		if len(body) > 2000 {
			body = body[:2000]
		}
		out = append(out, recall.Hit{Ring: "index", Kind: "indexed-file", ID: h.Ref, Form: "file", Cue: h.Record.Title, Gist: body, Full: h.Record.Body, Source: []recall.SourceRef{{URI: h.Record.Source.Ref}}, Status: "candidate", Score: h.Score, ObservedAt: h.Record.Updated, Why: []string{"indexed source", res.Mode}})
	}
	return out, nil
}
