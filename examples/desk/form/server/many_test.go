package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"
)

func TestPollPresetCLI(t *testing.T) {
	_, base := serveApp(t)
	addr := strings.TrimPrefix(base, "http://")
	var out bytes.Buffer
	if code := runCLI([]string{"add", "--addr", addr, "--poll", "Lunch?", "--options", "a,b,c", "--json"}, &out); code != 0 {
		t.Fatalf("poll add: %d %s", code, out.String())
	}
	var created map[string]any
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)
	out.Reset()
	if code := runCLI([]string{"show", id, "--addr", addr, "--key", created["ownerKey"].(string), "--json"}, &out); code != 0 {
		t.Fatalf("poll show: %d %s", code, out.String())
	}
	var shown map[string]any
	json.Unmarshal(out.Bytes(), &shown)
	if shown["respondents"] != "many" || shown["definition"].([]any)[0].(map[string]any)["id"] != "vote" {
		t.Fatalf("poll preset: %v", shown)
	}
	field := shown["definition"].([]any)[0].(map[string]any)
	options := field["options"].([]any)
	if field["label"] != "Lunch?" || len(options) != 3 || options[0] != "a" || options[1] != "b" || options[2] != "c" {
		t.Fatalf("poll prompt/options changed: %v", field)
	}
}

func participant(t *testing.T, base string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	r, err := c.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	return c
}

func participantAction(t *testing.T, c *http.Client, base, body string) (int, map[string]any) {
	t.Helper()
	r, err := c.Post(base+"/api/action", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	raw, _ := io.ReadAll(r.Body)
	v := map[string]any{}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("invalid JSON: %s", raw)
	}
	return r.StatusCode, v
}

func TestManyPollVotesReplaceAndClose(t *testing.T) {
	_, base := serveApp(t)
	id, key := newForm(t, base, `{"action":"add","title":"Lunch?","respondents":"many","definition":[{"id":"vote","label":"Lunch?","kind":"choice","required":true,"options":["a","b","c"]}]}`)
	clients := []*http.Client{participant(t, base), participant(t, base), participant(t, base)}
	for i, vote := range []string{"a", "b", "a"} {
		code, v := participantAction(t, clients[i], base, `{"action":"answer","id":"`+id+`","complete":true,"answers":{"vote":"`+vote+`"}}`)
		if code != 200 {
			t.Fatalf("vote %d: %d %v", i, code, v)
		}
	}
	show := func() map[string]any {
		code, raw := post(t, base, `{"action":"show","id":"`+id+`"}`, key)
		if code != 200 {
			t.Fatalf("show: %d %s", code, raw)
		}
		v := map[string]any{}
		json.Unmarshal(raw, &v)
		return v["results"].(map[string]any)
	}
	results := show()
	if results["count"] != float64(3) || results["aggregate"].(map[string]any)["vote"].(map[string]any)["a"] != float64(2) {
		t.Fatalf("tally: %v", results)
	}
	code, _ := participantAction(t, clients[0], base, `{"action":"answer","id":"`+id+`","complete":true,"answers":{"vote":"c"}}`)
	if code != 200 {
		t.Fatalf("re-vote: %d", code)
	}
	results = show()
	votes := results["aggregate"].(map[string]any)["vote"].(map[string]any)
	if votes["a"] != float64(1) || votes["b"] != float64(1) || votes["c"] != float64(1) {
		t.Fatalf("re-vote tally: %v", votes)
	}
	code, _ = post(t, base, `{"action":"close","id":"`+id+`"}`, key)
	if code != 200 {
		t.Fatalf("close: %d", code)
	}
	code, _ = participantAction(t, clients[1], base, `{"action":"answer","id":"`+id+`","complete":true,"answers":{"vote":"c"}}`)
	if code != 409 {
		t.Fatalf("closed response: %d", code)
	}
}

func TestManyCloseTimeAndDefinition(t *testing.T) {
	_, base := serveApp(t)
	closeAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	id, _ := newForm(t, base, `{"action":"add","respondents":"many","close_at":"`+closeAt+`","definition":[{"id":"q1","label":"Question","kind":"yes-no"}]}`)
	code, _ := participantAction(t, participant(t, base), base, `{"action":"answer","id":"`+id+`","answers":{"q1":true}}`)
	if code != 409 {
		t.Fatalf("expired response: %d", code)
	}
	_, _, mode, err := ParseFormDefinition([]byte(`{"respondents":"many","fields":[{"id":"q1","label":"Question","kind":"yes-no"}]}`), "form.json")
	if err != nil || mode != "many" {
		t.Fatalf("definition: %q %v", mode, err)
	}
}
