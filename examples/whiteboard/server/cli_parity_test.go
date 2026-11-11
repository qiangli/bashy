package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCLIPageVerbs(t *testing.T) {
	var preview bytes.Buffer
	if code := runCLI([]string{"draw", "main", "--dry-run", "--json"}, &preview); code != 0 || !strings.Contains(preview.String(), `"dry_run":true`) { t.Fatalf("draw preview: %d %s", code, preview.String()) }
	srv, a := deskServer(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	run := func(args ...string) (int, map[string]any) {
		t.Helper()
		var out bytes.Buffer
		args = append([]string{"--addr", addr, "--json"}, args...)
		code := runCLI(args, &out)
		var obj map[string]any
		if err := json.Unmarshal(out.Bytes(), &obj); err != nil {
			t.Fatalf("%v: %s", args, out.String())
		}
		return code, obj
	}
	if code, v := run("draw", "main", "--points", `[{"x":1,"y":1},{"x":2,"y":2}]`); code != 0 || len(v["strokes"].([]any)) != 1 {
		t.Fatalf("draw: %d %v", code, v)
	}
	if code, v := run("clear", "main", "--key", a.boards["main"].Facilitator); code != 0 || len(v["strokes"].([]any)) != 0 {
		t.Fatalf("clear: %d %v", code, v)
	}
	if code, v := run("lock", "main", "--key", a.boards["main"].Facilitator); code != 0 || v["locked"] != true {
		t.Fatalf("lock: %d %v", code, v)
	}
	if code, v := run("lock", "main", "--key", a.boards["main"].Facilitator, "--locked=false"); code != 0 || v["locked"] != false {
		t.Fatalf("unlock: %d %v", code, v)
	}
	if code, v := run("download", "main", "--format", "json"); code != 0 || v["board"] != "main" {
		t.Fatalf("download json: %d %v", code, v)
	}
	if code, v := run("download", "main", "--format", "png"); code != 0 || v["data_base64"] == "" {
		t.Fatalf("download png: %d %v", code, v)
	}
	created := request(a, token(), "", "/api/action", map[string]any{"action": "add", "id": "private"})
	var item map[string]any
	json.Unmarshal(created.Body.Bytes(), &item)
	key := item["ownerKey"].(string)
	shared := request(a, token(), key, "/api/action", map[string]any{"action": "share", "id": "private"})
	var invite map[string]any
	json.Unmarshal(shared.Body.Bytes(), &invite)
	if code, _ := run("revoke", "private", "--key", key, "--target", invite["invite"].(string)); code != 0 {
		t.Fatal("revoke failed")
	}
	if code, v := run("draw", "private", "--points", `[{"x":1,"y":1},{"x":2,"y":2}]`); code != 1 || v["error"] == nil {
		t.Fatalf("denied: %d %v", code, v)
	}
}
