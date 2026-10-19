package agentos

import (
	"context"
	"errors"
	"fmt"
	"github.com/qiangli/yoke/pkg/indexer"
	"github.com/qiangli/yoke/pkg/kbmemory"
	"github.com/qiangli/yoke/pkg/recall"
	"mvdan.cc/sh/v3/interp"
	"os"
	"path/filepath"
	"sync"
	"time"
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

var indexVisits struct {
	sync.Mutex
	day  string
	seen map[string]bool
}

func ObserveIndexDirectory(path string) {
	if dryRunRequested() {
		return
	}
	home := os.Getenv("BASHY_HOME")
	if home == "" {
		h, e := os.UserHomeDir()
		if e != nil {
			return
		}
		home = filepath.Join(h, ".bashy")
	}
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	key := home + "\x00" + path
	indexVisits.Lock()
	defer indexVisits.Unlock()
	if indexVisits.day != day {
		indexVisits.day = day
		indexVisits.seen = map[string]bool{}
	}
	if indexVisits.seen[key] || len(indexVisits.seen) >= 1024 {
		return
	}
	if indexer.RecordVisit(home, path, now) {
		indexVisits.seen[key] = true
	}
}
func indexVisitMiddleware(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		err := next(ctx, args)
		ObserveIndexDirectory(safeHandlerDir(ctx))
		return err
	}
}
