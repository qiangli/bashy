package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestReplyRoundTrip(t *testing.T) {
	a, base := serveApp(t)
	var out bytes.Buffer
	code := runCLI([]string{"reply", "--addr", strings.TrimPrefix(base, "http://"), "--extract", "testdata/extract.json", "--values", "testdata/values.json", "--refusals", "testdata/refusals.json", "--json"}, &out)
	if code != 0 {
		t.Fatalf("reply: %d %s", code, out.String())
	}
	var created map[string]any
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, key := created["id"].(string), created["ownerKey"].(string)
	status, raw := post(t, base, `{"action":"get","id":"`+id+`"}`, key)
	if status != 200 {
		t.Fatalf("get: %d %s", status, raw)
	}
	var got struct {
		Answers map[string]struct {
			Status, Selector, Label, Value, Reason string
			Done                                   bool
		}
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Answers) != 5 || got.Answers["person:1"].Label != "Full name" || got.Answers["#country"].Label != "Country" {
		t.Fatalf("rows: %s", raw)
	}
	if got.Answers["ssn"].Status != "needs-human" || got.Answers["ssn"].Reason != "field:label-ssn" || got.Answers["bio"].Status != "failed" || got.Answers["filled"].Status != "filled-by-agent" {
		t.Fatalf("statuses: %s", raw)
	}
	if got.Answers["bio"].Selector != `iframe[name="profile"] >>> textarea[name="bio"]` {
		t.Fatalf("selector: %s", raw)
	}
	disk, err := os.ReadFile(itemPath(a.dir, id))
	if err != nil {
		t.Fatal(err)
	}
	page := string(a.page(httptest.NewRequest("GET", "/?id="+id, nil)))
	for _, s := range []string{string(raw), string(disk), page} {
		if strings.Contains(s, "SENSITIVE") {
			t.Fatal("sensitive value leaked")
		}
	}
	if !strings.Contains(page, "data-desk-copy") || !strings.Contains(page, "field:label-ssn") || !strings.Contains(page, "Mark done") {
		t.Fatal("missing reply UI")
	}
	_, opened := post(t, base, `{"action":"open","id":"`+id+`"}`, "")
	if strings.Contains(string(opened), "Ada") || strings.Contains(string(opened), "answers") {
		t.Fatal("open leaks")
	}
	status, raw = post(t, base, `{"action":"answer","id":"`+id+`","answers":{"ssn":true}}`, "")
	if status != 200 {
		t.Fatalf("mark done: %d %s", status, raw)
	}
	a.items = map[string]*item{}
	_, raw = post(t, base, `{"action":"get","id":"`+id+`"}`, key)
	json.Unmarshal(raw, &got)
	if !got.Answers["ssn"].Done || got.Answers["ssn"].Status != "needs-human" {
		t.Fatalf("completion lost: %s", raw)
	}
	status, _ = post(t, base, `{"action":"answer","id":"`+id+`","answers":{"ssn":"SENSITIVE"}}`, "")
	if status != 400 {
		t.Fatal("reply accepted a value as completion")
	}
}

func TestReplyValidationAndStdin(t *testing.T) {
	extract, err := os.ReadFile("testdata/extract.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                      string
		extract, values, refusals string
	}{
		{"object data", `{"data":{"elements":[]}}`, "", ""},
		{"missing elements", `{"data":"{}"}`, "", ""},
		{"duplicate identity", `{"data":"{\"elements\":[{\"tag\":\"input\",\"stable_id\":\"x\"},{\"tag\":\"input\",\"stable_id\":\"x\"}]}"}`, "", ""},
		{"unknown refusal", string(extract), "", `{"missing":{"outcome_class":"refused"}}`},
		{"invalid proposal status", string(extract), `{"person:1":{"value":"SENSITIVE","status":"failed"}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildReply([]byte(tc.extract), []byte(tc.values), []byte(tc.refusals))
			if err == nil {
				t.Fatal("accepted invalid reply input")
			}
			if strings.Contains(err.Error(), "SENSITIVE") {
				t.Fatal("error leaked proposal")
			}
		})
	}
	file, err := os.Open("testdata/extract.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	previous := os.Stdin
	os.Stdin = file
	defer func() { os.Stdin = previous }()
	base := startServer(t)
	var out bytes.Buffer
	if code := runCLI([]string{"reply", "--addr", strings.TrimPrefix(base, "http://"), "--extract", "-"}, &out); code != 0 {
		t.Fatalf("stdin: %d %s", code, out.String())
	}
	if strings.Contains(out.String(), "Full name") {
		t.Fatal("create prints content")
	}
}

func TestReplyCompletionAndImmutability(t *testing.T) {
	_, base := serveApp(t)
	extract, _ := os.ReadFile("testdata/extract.json")
	payload, _ := json.Marshal(map[string]any{"action": "reply", "extract": json.RawMessage(extract)})
	id, key := newForm(t, base, string(payload))
	code, _ := post(t, base, `{"action":"answer","id":"`+id+`","complete":true}`, "")
	if code != 400 {
		t.Fatal("incomplete reply submitted")
	}
	code, _ = post(t, base, `{"action":"put","id":"`+id+`","answers":{"ssn":"SENSITIVE"}}`, key)
	if code != 409 {
		t.Fatal("reply mutated through put")
	}
	code, raw := post(t, base, `{"action":"answer","id":"`+id+`","complete":true,"answers":{"person:1":true,"ssn":true,"bio":true,"#country":true,"filled":true}}`, "")
	if code != 200 || !strings.Contains(string(raw), `"answered"`) {
		t.Fatalf("complete: %d %s", code, raw)
	}
	post(t, base, `{"action":"close","id":"`+id+`"}`, key)
	code, _ = post(t, base, `{"action":"answer","id":"`+id+`","answers":{"ssn":false}}`, "")
	if code != 409 {
		t.Fatal("closed reply accepted completion")
	}
}
