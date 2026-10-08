// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestExplainGoReachesTheDeltaTable(t *testing.T) {
	var out, errb bytes.Buffer
	if code := dispatchExplain([]string{"go", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	var tbl struct {
		Rows []struct{ ID, Workaround string } `json:"rows"`
	}
	if err := json.Unmarshal(out.Bytes(), &tbl); err != nil || len(tbl.Rows) == 0 {
		t.Fatalf("json: %v rows=%d", err, len(tbl.Rows))
	}

	// A refusal line resolves to its row, and the answer names the workaround.
	out.Reset()
	refusal := "script.bsh: line 1: package must begin a Go compilation unit"
	if code := dispatchExplain([]string{"go", refusal}, &out, &errb); code != 0 {
		t.Fatalf("refusal lookup exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "M15") || !strings.Contains(out.String(), "workaround:") {
		t.Fatalf("refusal did not resolve to its row and workaround:\n%s", out.String())
	}

	if code := dispatchExplain([]string{"python"}, &out, &errb); code != 2 {
		t.Fatalf("unknown topic exit %d, want 2", code)
	}
}

func TestExplainIsACatalogedVerb(t *testing.T) {
	_, _, verbs := commandsCatalog()
	for _, v := range verbs {
		if v == "explain" {
			if rec := verbAtlasRecord("explain", false); rec.Synopsis == "" {
				t.Fatal("explain has no synopsis")
			}
			return
		}
	}
	t.Fatal("explain verb not in commandsCatalog()")
}
