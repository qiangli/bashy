package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	kit "desk-kit"
)

// The desk agent door: `brainstorm-server cli [flags] new|open|get|put|list|share|close`.
// It speaks the same /api/action route as the page. open/new/put/share/close
// print id, URL and status only; get prints the session content as JSON.
func runCLI(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("brainstorm", flag.ContinueOnError)
	fs.SetOutput(stdout)
	addr := fs.String("addr", "127.0.0.1:18091", "server address")
	prefix := fs.String("prefix", "", "URL prefix when served behind a sub-path")
	asJSON := fs.Bool("json", false, "print the raw JSON API object")
	title := fs.String("title", "", "accepted for desk compatibility; session title (new)")
	body := fs.String("body", "", "idea text (new, put)")
	idFlag := fs.String("id", "", "session id (new)")
	key := fs.String("key", "", "facilitator key (BRAINSTORM_KEY also works)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stdout, "Usage: brainstorm [serve|new|open|get|put|list|share|close] [--json]")
		return 2
	}
	var positional []string
	rest := fs.Args()
	for len(rest) > 0 {
		positional = append(positional, rest[0])
		if err := fs.Parse(rest[1:]); err != nil {
			return 2
		}
		rest = fs.Args()
	}
	verb := positional[0]
	positional = positional[1:]
	if !deskActions[verb] {
		fmt.Fprintf(stdout, "error: unknown verb %q\n", verb)
		return 2
	}
	if *key == "" {
		*key = os.Getenv("BRAINSTORM_KEY")
	}
	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		fmt.Fprintf(stdout, "error: bad -addr %q\n", *addr)
		return 2
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	base := "http://" + net.JoinHostPort(host, port) + *prefix

	payload := map[string]any{"action": verb}
	switch verb {
	case "list":
	case "new":
		payload["id"], payload["title"], payload["body"] = *idFlag, *title, *body
	default:
		if len(positional) == 0 {
			fmt.Fprintln(stdout, "error: give the session id")
			return 2
		}
		if !kit.ValidName(positional[0]) {
			fmt.Fprintln(stdout, "error: use 1-64 letters, digits, - or _ for an id")
			return 2
		}
		payload["id"] = positional[0]
		if verb == "put" {
			payload["body"] = *body
		}
	}
	raw, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", base+"/api/action", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if *key != "" {
		req.Header.Set("X-Facilitator-Key", *key)
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}}
	r, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(stdout, "error: brainstorm server at %s is not reachable: %v\n", base, err)
		return 1
	}
	defer r.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if r.StatusCode != 200 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(buf, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(buf))
		}
		fmt.Fprintf(stdout, "error: %s\n", e.Error)
		return 1
	}
	if verb == "list" {
		var arr []map[string]any
		if json.Unmarshal(buf, &arr) != nil {
			fmt.Fprintln(stdout, "error: invalid server response")
			return 1
		}
		if *asJSON {
			fmt.Fprintln(stdout, kit.CanonicalJSON(arr))
			return 0
		}
		for _, e := range arr {
			fmt.Fprintf(stdout, "%v %v %v\n", e["id"], e["status"], e["url"])
		}
		return 0
	}
	var obj map[string]any
	if json.Unmarshal(buf, &obj) != nil {
		fmt.Fprintln(stdout, "error: invalid server response")
		return 1
	}
	if *asJSON || verb == "get" {
		fmt.Fprintln(stdout, kit.CanonicalJSON(obj))
		return 0
	}
	fmt.Fprintf(stdout, "%v %v %v\n", obj["id"], obj["url"], obj["status"])
	if inv, ok := obj["invite"]; ok {
		fmt.Fprintf(stdout, "invite: %v\n", inv)
	}
	if k, ok := obj["ownerKey"]; ok {
		fmt.Fprintf(stdout, "ownerKey: %v (keep private)\n", k)
	}
	return 0
}
