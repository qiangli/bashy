// The memo agent door: new|open|show|set|list|share|close over the same
// JSON API the page uses. --json prints the raw API object; without it,
// open/new print only id, URL and status — never content.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	kit "desk-kit"
)

func runCLI(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("memo", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:18097", "tool server address")
	data := fs.String("data", kit.DefaultDataDir("memo"), "data directory")
	prefix := fs.String("prefix", "", "URL prefix when served behind a sub-path")
	asJSON := fs.Bool("json", false, "print the raw JSON API object")
	title := fs.String("title", "", "item title")
	body := fs.String("body", "", "item body")
	idFlag := fs.String("id", "", "item id (new)")
	key := fs.String("key", "", "owner key (MEMO_KEY also works)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *key == "" {
		*key = os.Getenv("MEMO_KEY")
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(stdout, "Usage: memo [serve|add|open|show|set|list|share|close] [--json]")
		return 2
	}
	verb := rest[0]
	_positional := rest[1:]
	if verb == "serve" {
		sfs := flag.NewFlagSet("serve", flag.ContinueOnError)
		breakFlag := sfs.String("break", "", "deliberately break one contract point")
		_ = sfs.Parse(_positional)
		breakMode = *breakFlag
		serve(*addr, *data, *prefix)
		return 0
	}
	base := "http://" + *addr + *prefix
	call := func(action string, payload map[string]any) (map[string]any, []map[string]any, int) {
		payload["action"] = action
		raw, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", base+"/api/action", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if *key != "" {
			req.Header.Set("X-Facilitator-Key", *key)
		}
		client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}}
		r, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(stdout, "error: memo server at %s is not reachable: %v\n", base, err)
			return nil, nil, -1
		}
		defer r.Body.Close()
		buf, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if r.StatusCode != 200 {
			var e struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal(buf, &e)
			if e.Error == "" {
				e.Error = strings.TrimSpace(string(buf))
			}
			fmt.Fprintf(stdout, "error: %s\n", e.Error)
			return nil, nil, 1
		}
		var arr []map[string]any
		if err := json.Unmarshal(buf, &arr); err == nil {
			return nil, arr, 0
		}
		var obj map[string]any
		if err := json.Unmarshal(buf, &obj); err != nil {
			fmt.Fprintf(stdout, "error: invalid server response\n")
			return nil, nil, 1
		}
		return obj, nil, 0
	}
	emit := func(obj map[string]any, arr []map[string]any) int {
		if *asJSON {
			if arr != nil {
				fmt.Fprintln(stdout, kit.CanonicalJSON(arr))
			} else {
				fmt.Fprintln(stdout, kit.CanonicalJSON(obj))
			}
			return 0
		}
		if arr != nil {
			for _, e := range arr {
				fmt.Fprintf(stdout, "%v %v %v\n", e["id"], e["status"], e["url"])
			}
			return 0
		}
		if _, isList := obj["items"]; isList {
			fmt.Fprintln(stdout, kit.CanonicalJSON(obj))
			return 0
		}
		switch verb {
		case "show":
			fmt.Fprintf(stdout, "%v\n%v\n", obj["title"], obj["body"])
		default:
			// new/open/put/share/close: id, URL, status only — never content.
			fmt.Fprintf(stdout, "%v %v %v\n", obj["id"], obj["url"], obj["status"])
			if inv, ok := obj["invite"]; ok {
				fmt.Fprintf(stdout, "invite: %v\n", inv)
			}
			if k, ok := obj["ownerKey"]; ok {
				fmt.Fprintf(stdout, "ownerKey: %v (keep private)\n", k)
			}
		}
		return 0
	}
	needID := func() (string, bool) {
		if len(_positional) == 0 {
			fmt.Fprintln(stdout, "error: give the item id")
			return "", false
		}
		if !kit.ValidName(_positional[0]) {
			fmt.Fprintln(stdout, "error: use 1-64 letters, digits, - or _ for an id")
			return "", false
		}
		return _positional[0], true
	}
	switch verb {
	case "add":
		obj, _, code := call("add", map[string]any{"id": *idFlag, "title": *title, "body": *body})
		if code != 0 {
			return code
		}
		return emit(obj, nil)
	case "open":
		id, ok := needID()
		if !ok {
			return 2
		}
		obj, _, code := call("open", map[string]any{"id": id})
		if code != 0 {
			return code
		}
		return emit(obj, nil)
	case "show":
		id, ok := needID()
		if !ok {
			return 2
		}
		obj, _, code := call("show", map[string]any{"id": id})
		if code != 0 {
			return code
		}
		return emit(obj, nil)
	case "set":
		id, ok := needID()
		if !ok {
			return 2
		}
		obj, _, code := call("set", map[string]any{"id": id, "title": *title, "body": *body})
		if code != 0 {
			return code
		}
		return emit(obj, nil)
	case "list":
		obj, arr, code := call("list", map[string]any{})
		if code != 0 {
			return code
		}
		return emit(obj, arr)
	case "share":
		id, ok := needID()
		if !ok {
			return 2
		}
		obj, _, code := call("share", map[string]any{"id": id})
		if code != 0 {
			return code
		}
		return emit(obj, nil)
	case "close":
		id, ok := needID()
		if !ok {
			return 2
		}
		obj, _, code := call("close", map[string]any{"id": id})
		if code != 0 {
			return code
		}
		return emit(obj, nil)
	case "join":
		if len(_positional) < 2 {
			fmt.Fprintln(stdout, "error: memo join ID INVITE")
			return 2
		}
		u := base + "/?id=" + url.QueryEscape(_positional[0]) + "#invite=" + _positional[1]
		fmt.Fprintln(stdout, u)
		return 0
	default:
		fmt.Fprintf(stdout, "error: unknown verb %q\n", verb)
		return 2
	}
}
