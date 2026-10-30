// The viewer agent door: the same verbs as POST /api/action, one to one.
// --json prints the API object; without it, add/append/open/share/close and
// list print only id, URL and status — never content.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	kit "desk-kit"
)

const usage = `Usage: viewer VERB [ID] [options]   (view-only: no set/edit — edit text in notepad)
  serve   [--addr HOST:PORT] [--dir DIR] [--prefix /P]      loopback server (default 127.0.0.1:18096)
  add     [ID] [--sprint N | --session S] [--title T] [CONTENT]   create a book (+ optional first page)
  append  ID [--title T] CONTENT                           add the next page to a book
  show    ID [--page N|pN|first|last]                      read pages (Markdown + template data)
  open    ID                                               id, URL, status only
  list | share ID | close ID | join ID INVITE
  export  ID [--page N] [--to html|pdf] [-o FILE]          self-contained HTML (PDF via headless Chrome)
CONTENT: --body MD | -f FILE|-  |  --template NAME --data FILE|-|JSON [--sprint N]   [--asset FILE]...
Templates: ` + "status, plan-summary, recap, close-out" + ` (shaped copies in DIR/templates/ override)
Common: --json --fields F --no-input --explain --dry-run --help=mcp`

type cliOptions struct {
	addr, dir, prefix, key, breakFlag string
	title, body, file, template, data string
	session, page, to, output         string
	sprint                            int
	assets                            []string
	asJSON                            bool
	verb                              string
	args                              []string
}

func parseCLI(raw []string) (*cliOptions, error) {
	o := &cliOptions{addr: "127.0.0.1:18096", dir: kit.DefaultDataDir(tool), key: os.Getenv("VIEWER_KEY")}
	if v := os.Getenv("VIEWER_ADDR"); v != "" {
		o.addr = v
	}
	if v := os.Getenv("VIEWER_DATA"); v != "" {
		o.dir = v
	}
	o.prefix = os.Getenv("VIEWER_PREFIX")
	var pos []string
	for i := 0; i < len(raw); i++ {
		arg := raw[i]
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			pos = append(pos, arg)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		value := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 < len(raw) {
				i++
				return raw[i], nil
			}
			return "", fmt.Errorf("%s needs a value", arg)
		}
		var err error
		var s string
		switch name {
		case "h", "help":
			return nil, errHelp
		case "json":
			o.asJSON = true
			continue
		case "addr", "dir", "prefix", "key", "break", "title", "body", "f", "file", "template", "data",
			"session", "page", "to", "o", "output", "sprint", "asset":
			s, err = value()
		default:
			return nil, fmt.Errorf("unknown option %s", arg)
		}
		if err != nil {
			return nil, err
		}
		switch name {
		case "addr":
			o.addr = s
		case "dir":
			o.dir = s
		case "prefix":
			o.prefix = s
		case "key":
			o.key = s
		case "break":
			o.breakFlag = s
		case "title":
			o.title = s
		case "body":
			o.body = s
		case "f", "file":
			o.file = s
		case "template":
			o.template = s
		case "data":
			o.data = s
		case "session":
			o.session = s
		case "page":
			o.page = s
		case "to":
			o.to = s
		case "o", "output":
			o.output = s
		case "asset":
			o.assets = append(o.assets, s)
		case "sprint":
			n, err := strconv.Atoi(s)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("--sprint wants a positive number, got %q", s)
			}
			o.sprint = n
		}
	}
	if len(pos) > 0 {
		o.verb, o.args = pos[0], pos[1:]
	}
	return o, nil
}

var errHelp = fmt.Errorf("help")

func runCLI(args []string, stdin io.Reader, stdout io.Writer) int {
	args, common, err := kit.ParseCommon(args)
	if err != nil {
		return kit.UsageError(stdout, tool, false, err.Error())
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
		effects := "read"
		switch verb {
		case "add", "append", "share", "close", "join", "export":
			effects = "write"
		case "serve":
			effects = "net"
		}
		fmt.Fprintln(stdout, kit.CanonicalJSON(kit.Envelope(tool, map[string]any{"verb": verb, "effects": effects, "dry_run": common.DryRun, "view_only": true})))
		return 0
	}
	o, err := parseCLI(args)
	if err == errHelp {
		fmt.Fprintln(stdout, usage)
		return 0
	}
	if err != nil {
		return kit.UsageError(stdout, tool, common.JSON, err.Error())
	}
	o.asJSON = o.asJSON || common.JSON
	if o.verb == "" {
		fmt.Fprintln(stdout, usage)
		return 2
	}
	c := &client{o: o, out: stdout, fields: common.Fields}

	switch o.verb {
	case "serve":
		breakMode = o.breakFlag
		serve(o.addr, o.dir, o.prefix)
		return 0
	case "add":
		payload := map[string]any{"title": o.title}
		if len(o.args) > 0 {
			payload["id"] = o.args[0]
		}
		if o.sprint > 0 {
			payload["sprint"] = o.sprint
		}
		if o.session != "" {
			payload["session"] = o.session
		}
		if err := o.content(payload, stdin, true); err != nil {
			return kit.UsageError(stdout, tool, o.asJSON, err.Error())
		}
		return c.do("add", payload)
	case "append":
		id, ok := c.needID()
		if !ok {
			return 2
		}
		payload := map[string]any{"id": id, "title": o.title}
		if o.sprint > 0 {
			payload["sprint"] = o.sprint
		}
		if err := o.content(payload, stdin, false); err != nil {
			return kit.UsageError(stdout, tool, o.asJSON, err.Error())
		}
		c.resolveKey(id)
		return c.do("append", payload)
	case "show":
		id, ok := c.needID()
		if !ok {
			return 2
		}
		c.resolveKey(id)
		return c.do("show", map[string]any{"id": id, "page": o.page})
	case "open", "share", "close":
		id, ok := c.needID()
		if !ok {
			return 2
		}
		c.resolveKey(id)
		return c.do(o.verb, map[string]any{"id": id})
	case "list":
		return c.do("list", map[string]any{})
	case "join":
		if len(o.args) < 2 {
			return kit.UsageError(stdout, tool, o.asJSON, "viewer join ID INVITE")
		}
		fmt.Fprintln(stdout, "http://"+o.addr+o.prefix+"/?id="+url.QueryEscape(o.args[0])+"#invite="+o.args[1])
		return 0
	case "export":
		id, ok := c.needID()
		if !ok {
			return 2
		}
		c.resolveKey(id)
		return c.export(id)
	case "set", "edit", "put":
		return kit.UsageError(stdout, tool, o.asJSON, "viewer is view-only: pages are never edited; add the next page with viewer append, or edit text in notepad")
	default:
		return kit.UsageError(stdout, tool, o.asJSON, fmt.Sprintf("unknown verb %q (viewer verbs: %s)", o.verb, strings.Join(kit.ToolVerbs(tool), ", ")))
	}
}

// content fills body/template/data/assets from the flags.
func (o *cliOptions) content(payload map[string]any, stdin io.Reader, optional bool) error {
	read := func(src string) ([]byte, error) {
		if src == "-" {
			return io.ReadAll(io.LimitReader(stdin, 8<<20))
		}
		return os.ReadFile(src)
	}
	body := o.body
	if o.file != "" {
		if body != "" {
			return fmt.Errorf("give --body or -f, not both")
		}
		raw, err := read(o.file)
		if err != nil {
			return err
		}
		body = string(raw)
	}
	if body != "" {
		payload["body"] = body
	}
	if o.template != "" {
		payload["template"] = o.template
	}
	if o.data != "" {
		var raw []byte
		trimmed := strings.TrimSpace(o.data)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			raw = []byte(trimmed) // inline JSON
		} else {
			b, err := read(o.data)
			if err != nil {
				return err
			}
			raw = b
		}
		if !json.Valid(raw) {
			return fmt.Errorf("--data %s is not valid JSON", o.data)
		}
		payload["data"] = json.RawMessage(raw)
	}
	if !optional && body == "" && o.template == "" {
		return fmt.Errorf("nothing to append: give --body MD, -f FILE, or --template NAME --data FILE")
	}
	var assets []assetIn
	for _, f := range o.assets {
		raw, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		assets = append(assets, assetIn{Name: filepath.Base(f), Data: base64.StdEncoding.EncodeToString(raw)})
	}
	if assets != nil {
		payload["assets"] = assets
	}
	return nil
}

type client struct {
	o      *cliOptions
	out    io.Writer
	fields string
}

func (c *client) base() string { return "http://" + c.o.addr + c.o.prefix }

func (c *client) needID() (string, bool) {
	if len(c.o.args) == 0 {
		kit.UsageError(c.out, tool, c.o.asJSON, "give the book id")
		return "", false
	}
	if !kit.ValidName(c.o.args[0]) {
		kit.UsageError(c.out, tool, c.o.asJSON, "use 1-64 letters, digits, - or _ for an id")
		return "", false
	}
	return c.o.args[0], true
}

// resolveKey reads the owner key from the local store when --key is
// omitted: the agent that created a book on this host can manage it.
func (c *client) resolveKey(id string) {
	if c.o.key != "" {
		return
	}
	if raw, err := os.ReadFile(bookPath(c.o.dir, id)); err == nil {
		var saved struct {
			Owner string `json:"owner"`
		}
		if json.Unmarshal(raw, &saved) == nil {
			c.o.key = saved.Owner
		}
	}
}

func (c *client) httpClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}
}

// call posts one action and returns the decoded object or an exit code.
func (c *client) call(action string, payload map[string]any) (map[string]any, int) {
	payload["action"] = action
	raw, _ := json.Marshal(payload)
	q := ""
	if c.fields != "" {
		q = "?fields=" + url.QueryEscape(c.fields)
	}
	req, _ := http.NewRequest("POST", c.base()+"/api/action"+q, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if c.o.key != "" {
		req.Header.Set("X-Facilitator-Key", c.o.key)
	}
	r, err := c.httpClient().Do(req)
	if err != nil {
		msg := fmt.Sprintf("viewer server at %s is not reachable (start it with viewer serve): %v", c.base(), err)
		if c.o.asJSON {
			fmt.Fprintln(c.out, kit.CanonicalJSON(kit.Envelope(tool, map[string]any{"error": map[string]any{"code": "unavailable", "message": msg}})))
		} else {
			fmt.Fprintln(c.out, "error:", msg)
		}
		return nil, 1
	}
	defer r.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if r.StatusCode != http.StatusOK {
		if c.o.asJSON {
			fmt.Fprintln(c.out, strings.TrimSpace(string(buf)))
		} else {
			fmt.Fprintln(c.out, "error:", kit.ErrorMessage(buf))
		}
		if r.StatusCode == http.StatusBadRequest {
			return nil, 2
		}
		return nil, 1
	}
	var obj map[string]any
	if err := json.Unmarshal(buf, &obj); err != nil {
		fmt.Fprintln(c.out, "error: invalid server response")
		return nil, 1
	}
	return obj, 0
}

func (c *client) do(action string, payload map[string]any) int {
	obj, code := c.call(action, payload)
	if code != 0 {
		return code
	}
	if c.o.asJSON {
		fmt.Fprintln(c.out, kit.CanonicalJSON(obj))
		return 0
	}
	switch action {
	case "list":
		items, _ := obj["items"].([]any)
		for _, e := range items {
			if m, ok := e.(map[string]any); ok {
				fmt.Fprintf(c.out, "%v %v %v pages=%v\n", m["id"], m["status"], m["url"], m["pages"])
			}
		}
	case "show":
		fmt.Fprintf(c.out, "# %v\n", obj["title"])
		if p, ok := obj["page"].(map[string]any); ok {
			fmt.Fprintf(c.out, "\n<!-- %v · %v -->\n%v\n", p["id"], p["title"], p["markdown"])
			return 0
		}
		items, _ := obj["items"].([]any)
		for _, e := range items {
			if p, ok := e.(map[string]any); ok {
				fmt.Fprintf(c.out, "\n<!-- %v · %v -->\n%v\n", p["id"], p["title"], p["markdown"])
			}
		}
	default:
		// add/append/open/share/close: id, URL, status only — never content.
		u := obj["url"]
		if pu, ok := obj["page_url"]; ok {
			u = pu
		}
		fmt.Fprintf(c.out, "%v %v %v\n", obj["id"], u, obj["status"])
		if inv, ok := obj["inviteURL"]; ok {
			fmt.Fprintf(c.out, "invite: %v\n", inv)
		}
		if k, ok := obj["ownerKey"]; ok {
			fmt.Fprintf(c.out, "ownerKey: %v (keep private)\n", k)
		}
	}
	return 0
}

// export: html to -o (fetched bytes) or the server's exports dir; pdf
// prints that HTML with headless Chrome.
func (c *client) export(id string) int {
	to := strings.ToLower(strings.TrimPrefix(c.o.to, "."))
	if to == "" {
		to = "html"
	}
	if to == "htm" {
		to = "html"
	}
	if to != "html" && to != "pdf" {
		return kit.UsageError(c.out, tool, c.o.asJSON, fmt.Sprintf("unsupported export format %q (html, pdf)", c.o.to))
	}
	if to == "html" && c.o.output == "" {
		return c.do("export", map[string]any{"id": id, "page": c.o.page})
	}
	u := c.base() + "/api/export?id=" + url.QueryEscape(id)
	if c.o.page != "" {
		u += "&page=" + url.QueryEscape(c.o.page)
	}
	req, _ := http.NewRequest("GET", u, nil)
	if c.o.key != "" {
		req.Header.Set("X-Facilitator-Key", c.o.key)
	}
	r, err := c.httpClient().Do(req)
	if err != nil {
		fmt.Fprintf(c.out, "error: viewer server at %s is not reachable: %v\n", c.base(), err)
		return 1
	}
	defer r.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if r.StatusCode != http.StatusOK {
		if c.o.asJSON {
			fmt.Fprintln(c.out, strings.TrimSpace(string(buf)))
		} else {
			fmt.Fprintln(c.out, "error:", kit.ErrorMessage(buf))
		}
		return 1
	}
	out := c.o.output
	if out == "" {
		out = id + "." + to
	}
	out, _ = filepath.Abs(out)
	if to == "pdf" {
		err = printPDF(buf, out)
	} else {
		err = os.WriteFile(out, buf, 0644)
	}
	if err != nil {
		fmt.Fprintln(c.out, "error:", err)
		return 1
	}
	if c.o.asJSON {
		fmt.Fprintln(c.out, kit.CanonicalJSON(kit.Envelope(tool, map[string]any{"id": id, "format": to, "path": out})))
	} else {
		fmt.Fprintln(c.out, out)
	}
	return 0
}
