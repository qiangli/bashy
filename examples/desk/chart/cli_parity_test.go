package main

import (
	"bytes"
	kit "desk-kit"
	"encoding/json"
	"strings"
	"testing"
)

func TestCLIJoinPageAction(t *testing.T) {
	var preview bytes.Buffer
	if code := cli([]string{"join", "join-cli", "--dry-run", "--json"}, strings.NewReader(""), &preview); code != 0 || !strings.Contains(preview.String(), `"dry_run":true`) { t.Fatalf("join preview: %d %s", code, preview.String()) }
	_, srv := startServer(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	run := func(args ...string) (int, map[string]any) {
		t.Helper()
		var out bytes.Buffer
		args = append(args, "--addr", addr, "--json")
		code := cli(args, strings.NewReader("[1,2]"), &out)
		var v map[string]any
		if err := json.Unmarshal(out.Bytes(), &v); err != nil {
			t.Fatalf("%v: %s", args, out.String())
		}
		return code, v
	}
	code, created := run("add", "--id", "join-cli", "--input", "-", "--widget", "gauge")
	if code != 0 {
		t.Fatalf("add: %v", created)
	}
	key := created["ownerKey"].(string)
	code, shared := run("share", "join-cli", "--key", key)
	if code != 0 {
		t.Fatalf("share: %v", shared)
	}
	participant := kit.NewToken()
	code, joined := run("join", "join-cli", "--invite", shared["invite"].(string), "--participant", participant)
	if code != 0 || joined["id"] != "join-cli" {
		t.Fatalf("join: %d %v", code, joined)
	}
	code, deniedJoin := run("join", "join-cli", "--invite", shared["invite"].(string), "--participant", kit.NewToken())
	if code != 1 || deniedJoin["error"] == nil {
		t.Fatalf("reused invite: %d %v", code, deniedJoin)
	}
	code, shown := run("show", "join-cli", "--participant", participant)
	if code != 0 || shown["id"] != "join-cli" {
		t.Fatalf("member show: %d %v", code, shown)
	}
	code, denied := run("show", "join-cli", "--participant", kit.NewToken())
	if code != 1 || denied["error"] == nil {
		t.Fatalf("denied: %d %v", code, denied)
	}
}
