package main

import (
 "bytes"
 "encoding/json"
 "strings"
 "testing"
)

func TestCLIPageVerbs(t *testing.T) {
	var preview bytes.Buffer
	if code := runCLI([]string{"vote", "ideas", "--dry-run", "--json"}, &preview); code != 0 || !strings.Contains(preview.String(), `"dry_run":true`) { t.Fatalf("vote preview: %d %s", code, preview.String()) }
 srv, a := deskServer(t)
 addr := strings.TrimPrefix(srv.URL, "http://")
 who := token()
 run := func(args ...string) (int, map[string]any) {
  t.Helper(); var out bytes.Buffer
  args = append([]string{"--addr", addr, "--json", "--participant", who}, args...)
  code := runCLI(args, &out)
  var obj map[string]any
  if err := json.Unmarshal(out.Bytes(), &obj); err != nil { t.Fatalf("%v: %s", args, out.String()) }
  return code, obj
 }
 code, created := run("add", "--id", "ideas")
 if code != 0 { t.Fatalf("add: %v", created) }
 key := created["ownerKey"].(string)
 card := request(a, who, key, "ideas", "/api/action", map[string]any{"action":"add", "session":"ideas", "text":"first", "column":0})
 good(t, card)
 var state map[string]any; json.Unmarshal(card.Body.Bytes(), &state)
 cardID := state["cards"].([]any)[0].(map[string]any)["id"].(string)
 if c, v := run("edit", "ideas", "--key", key, "--card", cardID, "--body", "edited"); c != 0 || !strings.Contains(v["cards"].([]any)[0].(map[string]any)["text"].(string), "edited") { t.Fatalf("edit: %d %v", c, v) }
 if c, v := run("delete", "ideas", "--key", key, "--card", cardID); c != 0 || len(v["cards"].([]any)) != 0 { t.Fatalf("delete: %d %v", c, v) }
 card = request(a, who, key, "ideas", "/api/action", map[string]any{"action":"add", "session":"ideas", "text":"vote me", "column":0})
 good(t, card); json.Unmarshal(card.Body.Bytes(), &state)
 cardID = state["cards"].([]any)[0].(map[string]any)["id"].(string)
 if c, _ := run("set", "ideas", "--key", key, "--phase", "discuss"); c != 0 { t.Fatal("discuss") }
 if c, _ := run("set", "ideas", "--key", key, "--phase", "vote"); c != 0 { t.Fatal("vote phase") }
 if c, v := run("vote", "ideas", "--key", key, "--card", cardID); c != 0 || v["cards"] == nil { t.Fatalf("vote: %d %v", c, v) }
 if c, v := run("download", "ideas", "--key", key, "--format", "json"); c != 0 || v["session"] != "ideas" { t.Fatalf("json export: %d %v", c, v) }
 if c, v := run("download", "ideas", "--key", key, "--format", "markdown"); c != 0 || v["content"] == "" { t.Fatalf("markdown export: %d %v", c, v) }
 shared := request(a, who, key, "ideas", "/api/action", map[string]any{"action":"share", "id":"ideas"})
 good(t, shared); var invite map[string]any; json.Unmarshal(shared.Body.Bytes(), &invite)
 if c, v := run("revoke", "ideas", "--key", key, "--target", invite["inviteCode"].(string)); c != 0 { t.Fatalf("revoke: %v", v) }
 if c, v := run("vote", "ideas", "--card", cardID, "--participant", token()); c != 1 || v["error"] == nil { t.Fatalf("denied: %d %v", c, v) }
}
