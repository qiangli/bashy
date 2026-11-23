// The page agent door: add|open|show|set|list|share|close|export over the
// same JSON API the page uses. --json prints the API object; without it,
// add/open/set/share/close/list print only id, URL and status — never
// content. The organizer's owner key comes from --key, PAGE_KEY or the
// local store (the CLI runs on the organizer's host).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	kit "desk-kit"
)

const usage = "Usage: page [serve|add|open|show|set|list|share|close|export|publish-outpost] [--json] [-f FILE|-] [--title T] [--audience host|lan|satellite] [--to md|html] [-o FILE]"

type cliOptions struct {
	addr, data, prefix, key         string
	title, body, file, id, audience string
	to, output, breakFlag           string
	asJSON, lan                     bool
	verb                            string
	args                            []string
}

// valueFlags take a value (--name V or --name=V); boolFlags do not.
var (
	valueFlags = map[string]bool{"addr": true, "data": true, "prefix": true, "key": true,
		"title": true, "body": true, "f": true, "file": true, "id": true, "audience": true,
		"to": true, "o": true, "output": true, "break": true}
	boolFlags = map[string]bool{"json": true, "lan": true}
)

func parseCLIOptions(raw []string) (*cliOptions, error) {
	o := &cliOptions{addr: envOr("PAGE_ADDR", "127.0.0.1:18113"),
		data: envOr("PAGE_DATA", kit.DefaultDataDir(tool)), prefix: os.Getenv("PAGE_PREFIX"),
		key: os.Getenv("PAGE_KEY")}
	var pos []string
	for i := 0; i < len(raw); i++ {
		arg := raw[i]
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			pos = append(pos, arg)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch {
		case boolFlags[name] && !hasVal:
			if name == "json" {
				o.asJSON = true
			} else {
				o.lan = true
			}
			continue
		case !valueFlags[name]:
			return nil, fmt.Errorf("unknown flag %s", arg)
		case !hasVal:
			if i+1 == len(raw) {
				return nil, fmt.Errorf("%s needs a value", arg)
			}
			i++
			val = raw[i]
		}
		switch name {
		case "addr":
			o.addr = val
		case "data":
			o.data = val
		case "prefix":
			o.prefix = val
		case "key":
			o.key = val
		case "title":
			o.title = val
		case "body":
			o.body = val
		case "f", "file":
			o.file = val
		case "id":
			o.id = val
		case "audience":
			o.audience = val
		case "to":
			o.to = val
		case "o", "output":
			o.output = val
		case "break":
			o.breakFlag = val
		}
	}
	if len(pos) > 0 {
		o.verb, o.args = pos[0], pos[1:]
	}
	return o, nil
}

func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func runCLI(args []string, stdin io.Reader, stdout io.Writer) int {
	args, common, err := kit.ParseCommon(args)
	if err != nil {
		fmt.Fprintln(stdout, "error:", err)
		return 2
	}
	if common.HelpMCP {
		fmt.Fprintln(stdout, kit.CanonicalJSON(kit.MCPHelp(tool)))
		return 0
	}
	if common.Explain || common.DryRun {
		verb := kit.FindVerb(tool, args)
		if verb == "" {
			return kit.UsageError(stdout, tool, true, "unknown verb")
		}
		fmt.Fprintln(stdout, kit.CanonicalJSON(kit.Envelope(tool, map[string]any{"verb": verb, "effects": "desk action", "dry_run": common.DryRun})))
		return 0
	}
	o, err := parseCLIOptions(args)
	if err != nil {
		return kit.UsageError(stdout, tool, common.JSON, err.Error())
	}
	o.asJSON = o.asJSON || common.JSON
	if o.verb == "" {
		fmt.Fprintln(stdout, usage)
		return 2
	}
	if o.verb == "serve" {
		breakMode = o.breakFlag
		serve(o.addr, o.data, o.prefix, o.lan)
		return 0
	}
	c := &client{o: o, fields: common.Fields, stdout: stdout}

	needID := func() (string, bool) {
		id := o.id
		if len(o.args) > 0 {
			id = o.args[0]
		}
		if !kit.ValidName(id) {
			kit.UsageError(stdout, tool, o.asJSON, "give the page id (1-64 letters, digits, - or _)")
			return "", false
		}
		return id, true
	}
	content := func() (string, bool) {
		if o.file == "" {
			return o.body, true
		}
		var raw []byte
		var err error
		if o.file == "-" {
			raw, err = io.ReadAll(stdin)
		} else {
			raw, err = os.ReadFile(o.file)
		}
		if err != nil {
			fmt.Fprintln(stdout, "error:", err)
			return "", false
		}
		return string(raw), true
	}

	switch o.verb {
	case "add":
		body, ok := content()
		if !ok {
			return 1
		}
		id := o.id
		if id == "" && len(o.args) > 0 {
			id = o.args[0]
		}
		return c.do("add", map[string]any{"id": id, "title": o.title, "body": body, "audience": o.audience})
	case "set":
		id, ok := needID()
		if !ok {
			return 2
		}
		body, ok := content()
		if !ok {
			return 1
		}
		c.resolveKey(id)
		return c.do("set", map[string]any{"id": id, "title": o.title, "body": body, "audience": o.audience})
	case "open", "show", "share", "close":
		id, ok := needID()
		if !ok {
			return 2
		}
		c.resolveKey(id)
		return c.do(o.verb, map[string]any{"id": id})
	case "list":
		return c.do("list", map[string]any{})
	case "export":
		id, ok := needID()
		if !ok {
			return 2
		}
		switch o.to {
		case "md", "markdown", "html":
		default:
			return kit.UsageError(stdout, tool, o.asJSON, "export to md or html (--to md|html)")
		}
		c.resolveKey(id)
		return c.export(id)
	default:
		return kit.UsageError(stdout, tool, o.asJSON, fmt.Sprintf("unknown verb %q", o.verb))
	}
}

type client struct {
	o      *cliOptions
	fields string
	stdout io.Writer
}

func (c *client) base() string { return "http://" + c.o.addr + c.o.prefix }

// resolveKey reads the owner key from the local store when none was given.
func (c *client) resolveKey(id string) {
	if c.o.key != "" {
		return
	}
	raw, err := os.ReadFile(itemPath(c.o.data, id))
	if err != nil {
		return
	}
	var saved struct {
		Owner string `json:"owner"`
	}
	if json.Unmarshal(raw, &saved) == nil {
		c.o.key = saved.Owner
	}
}

func (c *client) send(req *http.Request) (*http.Response, []byte, bool) {
	if c.o.key != "" {
		req.Header.Set("X-Facilitator-Key", c.o.key)
	}
	hc := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}
	r, err := hc.Do(req)
	if err != nil {
		fmt.Fprintf(c.stdout, "error: page server at %s is not reachable: %v\n", c.base(), err)
		return nil, nil, false
	}
	defer r.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	return r, buf, true
}

// fail prints a refusal and maps it to the exit code: 2 usage, 1 otherwise.
func (c *client) fail(status int, buf []byte) int {
	if c.o.asJSON {
		fmt.Fprintln(c.stdout, strings.TrimSpace(string(buf)))
	} else {
		fmt.Fprintln(c.stdout, "error:", kit.ErrorMessage(buf))
	}
	if status == http.StatusBadRequest {
		return 2
	}
	return 1
}

func (c *client) do(action string, payload map[string]any) int {
	payload["action"] = action
	for k, v := range payload {
		if v == "" {
			delete(payload, k)
		}
	}
	raw, _ := json.Marshal(payload)
	q := ""
	if c.fields != "" {
		q = "?fields=" + url.QueryEscape(c.fields)
	}
	req, _ := http.NewRequest("POST", c.base()+"/api/action"+q, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r, buf, ok := c.send(req)
	if !ok {
		return 1
	}
	if r.StatusCode != http.StatusOK {
		return c.fail(r.StatusCode, buf)
	}
	var obj map[string]any
	if err := json.Unmarshal(buf, &obj); err != nil {
		fmt.Fprintln(c.stdout, "error: invalid server response")
		return 1
	}
	if c.o.asJSON {
		fmt.Fprintln(c.stdout, kit.CanonicalJSON(obj))
		return 0
	}
	if items, ok := obj["items"].([]any); ok {
		for _, e := range items {
			if m, ok := e.(map[string]any); ok {
				fmt.Fprintf(c.stdout, "%v %v %v\n", m["id"], m["status"], m["url"])
			}
		}
		return 0
	}
	if action == "show" {
		fmt.Fprintf(c.stdout, "%v\n%v\n", obj["title"], obj["body"])
		return 0
	}
	// add/open/set/share/close: id, URL, status only — never content.
	fmt.Fprintf(c.stdout, "%v %v %v\n", obj["id"], obj["url"], obj["status"])
	if v, ok := obj["inviteURL"]; ok {
		fmt.Fprintf(c.stdout, "invite: %v\n", v)
	}
	if k, ok := obj["ownerKey"]; ok {
		fmt.Fprintf(c.stdout, "ownerKey: %v (keep private; also kept in the local store)\n", k)
	}
	return 0
}

// export fetches the record and writes it to -o FILE or stdout.
func (c *client) export(id string) int {
	to := c.o.to
	if to == "markdown" {
		to = "md"
	}
	req, _ := http.NewRequest("GET", c.base()+"/api/export?id="+url.QueryEscape(id)+"&to="+to, nil)
	r, buf, ok := c.send(req)
	if !ok {
		return 1
	}
	if r.StatusCode != http.StatusOK {
		return c.fail(r.StatusCode, buf)
	}
	out := map[string]any{"id": id, "format": to}
	if c.o.output != "" {
		path, err := filepath.Abs(c.o.output)
		if err == nil {
			err = os.WriteFile(path, buf, 0o644)
		}
		if err != nil {
			fmt.Fprintln(c.stdout, "error:", err)
			return 1
		}
		out["path"] = path
	} else if !c.o.asJSON {
		c.stdout.Write(buf)
		return 0
	} else {
		out["content"] = string(buf)
	}
	if c.o.asJSON {
		fmt.Fprintln(c.stdout, kit.CanonicalJSON(kit.Envelope(tool, out)))
	} else {
		fmt.Fprintln(c.stdout, out["path"])
	}
	return 0
}
