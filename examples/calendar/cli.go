// The calendar agent door: serve|add|show|set|list|cancel|open|share|join|close|
// export|import|save|load over the same JSON API the page uses, with
// bashy-calendar-v1 JSON envelopes, --dry-run zero-mutation guarantees,
// console runtime discovery via kit.DiscoverAppRuntime("calendar"),
// Git-directory save/load (never invoking git), and Google Calendar interop
// composing BASHY_GOOGLE_CMD.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	kit "github.com/qiangli/bashy/examples/desk/kit"
)

const usage = `Usage: calendar VERB [ID] [options]
  serve   [-addr HOST:PORT] [-data DIR] [-prefix /P]       loopback server (default 127.0.0.1:18099)
  add     [ID] [--title T] [--start RFC3339] [--end RFC3339] [--tz IANA_TZ]
          [--location LOC] [--body MD | -f FILE|-] [--ref REF]... [--audience PERSON]...
  show    ID [--tz IANA_TZ]                                read full event (including Markdown body)
  set     ID [--title T] [--start RFC3339] [--end RFC3339] [--tz IANA_TZ]
          [--location LOC] [--body MD | -f FILE|-] [--ref REF]... [--audience PERSON]...
          [--revoke MEMBER]
  list    [--from TIME] [--to TIME] [--tz IANA_TZ] [--audience PERSON] [--status STATUS]
  cancel  ID                                               mark cancelled (event kept)
  open    ID                                               id, URL, status only
  share   ID                                               mint single-use invite link
  join    ID INVITE                                        print browser join URL
  close   ID                                               mark closed
  export  [ID|--all] --to ics|json|google [-o FILE|-] [--calendar CAL_ID]
  import  [-f FILE|-] [--from ics|google] [--calendar CAL_ID]
  save    [--dir DIR]                                      write events.json + <id>.ics (default docs/calendar)
  load    [DIR] [--dir DIR]                                load events.json / *.ics idempotently
Common: --json --fields F --no-input --explain --dry-run --help=mcp`

type cliOptions struct {
	addr, data, prefix, publicURL string
	key, member, breakFlag        string
	handoff                       bool

	idFlag      string
	title       string
	titleSet    bool
	start       string
	startSet    bool
	end         string
	endSet      bool
	tz          string
	tzSet       bool
	location    string
	locationSet bool
	body        string
	bodySet     bool
	fileFlag    string
	refs        []string
	refsSet     bool
	audience    []string
	audienceSet bool
	status      string
	statusSet   bool
	revoke      string
	from        string
	to          string
	output      string
	gitDir      string
	calendarID  string
	all         bool
	asJSON      bool
	verb        string
	args        []string
}

var errHelp = errors.New("help")

func parseCLIOptions(raw []string) (*cliOptions, error) {
	o := &cliOptions{
		addr:      "127.0.0.1:18099",
		data:      kit.DefaultDataDir(tool),
		prefix:    os.Getenv("CALENDAR_PREFIX"),
		publicURL: os.Getenv("CALENDAR_PUBLIC_URL"),
		key:       os.Getenv("CALENDAR_KEY"),
		member:    os.Getenv("CALENDAR_MEMBER"),
	}
	if os.Getenv("BASHY_APP") == tool {
		if v := os.Getenv("BASHY_APP_ADDR"); v != "" {
			o.addr = v
			o.handoff = true
		}
		if v := os.Getenv("BASHY_APP_DATA"); v != "" {
			o.data = v
			o.handoff = true
		}
	}
	if v := os.Getenv("CALENDAR_ADDR"); v != "" {
		o.addr = v
		o.handoff = true
	}
	if v := os.Getenv("CALENDAR_DATA"); v != "" {
		o.data = v
		o.handoff = true
	}

	var pos []string
	for i := 0; i < len(raw); i++ {
		arg := raw[i]
		if arg == "--" {
			pos = append(pos, raw[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			pos = append(pos, arg)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		needVal := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 < len(raw) {
				i++
				return raw[i], nil
			}
			return "", fmt.Errorf("%s requires a value", arg)
		}

		switch name {
		case "h", "help":
			return nil, errHelp
		case "json":
			o.asJSON = true
			continue
		case "all":
			o.all = true
			continue
		}

		v, err := needVal()
		if err != nil {
			return nil, err
		}
		switch name {
		case "addr":
			o.addr = v
			o.handoff = true
		case "data":
			o.data = v
			o.handoff = true
		case "prefix":
			o.prefix = v
		case "public-url":
			o.publicURL = v
		case "key":
			o.key = v
		case "member":
			o.member = v
		case "break":
			o.breakFlag = v
		case "id":
			o.idFlag = v
		case "title":
			o.title = v
			o.titleSet = true
		case "start":
			o.start = v
			o.startSet = true
		case "end":
			o.end = v
			o.endSet = true
		case "tz":
			o.tz = v
			o.tzSet = true
		case "location":
			o.location = v
			o.locationSet = true
		case "body":
			o.body = v
			o.bodySet = true
		case "f", "file":
			o.fileFlag = v
		case "ref", "refs":
			o.refsSet = true
			for _, part := range strings.Split(v, ",") {
				if p := strings.TrimSpace(part); p != "" {
					o.refs = appendUnique(o.refs, p)
				}
			}
		case "audience":
			o.audienceSet = true
			for _, part := range strings.Split(v, ",") {
				if p := strings.TrimSpace(part); p != "" {
					o.audience = appendUnique(o.audience, p)
				}
			}
		case "status":
			o.status = v
			o.statusSet = true
		case "revoke":
			o.revoke = v
		case "from":
			o.from = v
		case "to":
			o.to = v
		case "o", "output":
			o.output = v
		case "dir":
			o.gitDir = v
		case "calendar":
			o.calendarID = v
		default:
			return nil, fmt.Errorf("unknown option %s", arg)
		}
	}

	if len(pos) > 0 {
		o.verb, o.args = pos[0], pos[1:]
	}
	return o, nil
}

func verbEffects(verb string) string {
	switch verb {
	case "show", "list", "open", "join":
		return "read"
	case "serve":
		return "net"
	case "import", "export":
		return "read,write,exec,net"
	default:
		return "read,write,net"
	}
}

func runCLI(args []string, stdout io.Writer) int {
	return runCLIWithIO(args, os.Stdin, stdout)
}

func runCLIWithIO(args []string, stdin io.Reader, stdout io.Writer) int {
	args, common, err := kit.ParseCommon(args)
	if err != nil {
		return kit.UsageError(stdout, tool, false, err.Error())
	}
	if common.HelpMCP {
		fmt.Fprintln(stdout, kit.CanonicalJSON(kit.MCPHelp(tool)))
		return 0
	}
	if common.Explain {
		verb := kit.FindVerb(tool, args)
		if verb == "" {
			return kit.UsageError(stdout, tool, true, "unknown verb")
		}
		return emitLocalEnvelope(stdout, true, common.Fields, map[string]any{
			"verb":    verb,
			"effects": verbEffects(verb),
			"dry_run": false,
		})
	}

	o, err := parseCLIOptions(args)
	if errors.Is(err, errHelp) {
		fmt.Fprintln(stdout, usage)
		return 0
	}
	if err != nil {
		return kit.UsageError(stdout, tool, common.JSON, err.Error())
	}
	o.asJSON = o.asJSON || common.JSON || common.DryRun

	if o.verb == "" {
		return kit.UsageError(stdout, tool, o.asJSON, "missing verb")
	}

	// Refuse legacy non-standard verbs immediately with exit 2 (contracttest).
	switch o.verb {
	case "serve", "add", "show", "set", "list", "cancel", "open", "share", "join", "close", "export", "import", "save", "load":
	default:
		return kit.UsageError(stdout, tool, o.asJSON, fmt.Sprintf("unknown verb %q (calendar verbs: %s)", o.verb, strings.Join(kit.ToolVerbs(tool), ", ")))
	}

	if o.verb == "serve" {
		if common.DryRun {
			return emitLocalEnvelope(stdout, true, common.Fields, map[string]any{
				"verb":    "serve",
				"effects": "net",
				"addr":    o.addr,
				"data":    o.data,
				"dry_run": true,
			})
		}
		breakMode = o.breakFlag
		serve(o.addr, o.data, o.prefix)
		return 0
	}

	// For read-only verbs under --dry-run (e.g. contracttest `--dry-run list`),
	// emit the versioned dry-run envelope directly with zero mutation.
	if common.DryRun && (o.verb == "list" || o.verb == "show" || o.verb == "open" || o.verb == "join") {
		return emitLocalEnvelope(stdout, true, common.Fields, map[string]any{
			"verb":    o.verb,
			"effects": verbEffects(o.verb),
			"dry_run": true,
		})
	}

	// Console runtime discovery per kb:enable-an-app-for-consumer-shaping:
	// explicit handoff wins; otherwise query `bashy app show calendar --runtime --json`;
	// never fall back silently to fixed port 127.0.0.1:18099.
	if !o.handoff {
		info, err := kit.DiscoverAppRuntime(tool)
		if err != nil {
			return emitFailure(stdout, o.asJSON, "runtime_unavailable", fmt.Sprintf("cannot discover calendar: %v", err), 1)
		}
		o.addr, o.data = info.Addr, info.Data
		if o.publicURL == "" {
			o.publicURL = info.URL
		}
	}

	c := &cliClient{o: o, stdin: stdin, out: stdout, fields: common.Fields, dryRun: common.DryRun}

	switch o.verb {
	case "add":
		return c.cmdAdd()
	case "open":
		id, ok := c.needID()
		if !ok {
			return 2
		}
		c.resolveKey(id)
		return c.doAction("open", map[string]any{"id": id})
	case "show":
		id, ok := c.needID()
		if !ok {
			return 2
		}
		c.resolveKey(id)
		payload := map[string]any{"id": id}
		if o.tzSet {
			payload["tz"] = o.tz
		}
		return c.doAction("show", payload)
	case "set":
		return c.cmdSet()
	case "cancel", "close", "share":
		id, ok := c.needID()
		if !ok {
			return 2
		}
		c.resolveKey(id)
		payload := map[string]any{"id": id}
		if c.dryRun {
			payload["dry_run"] = true
		}
		return c.doAction(o.verb, payload)
	case "list":
		return c.cmdList()
	case "join":
		return c.cmdJoin()
	case "export":
		return c.cmdExport()
	case "import":
		return c.cmdImport()
	case "save":
		return c.cmdSave()
	case "load":
		return c.cmdLoad()
	default:
		return kit.UsageError(stdout, tool, o.asJSON, fmt.Sprintf("unknown verb %q", o.verb))
	}
}

type cliClient struct {
	o      *cliOptions
	stdin  io.Reader
	out    io.Writer
	fields string
	dryRun bool
}

func (c *cliClient) base() string { return "http://" + c.o.addr + c.o.prefix }

func (c *cliClient) needID() (string, bool) {
	id := ""
	if len(c.o.args) > 0 {
		id = c.o.args[0]
	} else if c.o.idFlag != "" {
		id = c.o.idFlag
	}
	if id == "" {
		kit.UsageError(c.out, tool, c.o.asJSON, "give the event id")
		return "", false
	}
	if !kit.ValidName(id) {
		kit.UsageError(c.out, tool, c.o.asJSON, "use 1-64 letters, digits, - or _ for an id")
		return "", false
	}
	return id, true
}

// resolveKey auto-resolves the local owner key from opts.data/items/<id>.json
// when neither --key nor --member was explicitly supplied.
func (c *cliClient) resolveKey(id string) {
	if c.o.key != "" || c.o.member != "" || id == "" || c.o.data == "" {
		return
	}
	if raw, err := os.ReadFile(eventPath(c.o.data, id)); err == nil {
		var saved struct {
			Owner string `json:"owner"`
		}
		if json.Unmarshal(raw, &saved) == nil && saved.Owner != "" {
			c.o.key = saved.Owner
		}
	}
}

// resolveAllKeys loads local owner keys from opts.data/items/*.json for
// multi-item owner CLI verbs (list, export --all, save, import, load).
func (c *cliClient) resolveAllKeys() {
	if c.o.key != "" || c.o.member != "" || c.o.data == "" {
		return
	}
	files, err := filepath.Glob(filepath.Join(c.o.data, "items", "*.json"))
	if err != nil || len(files) == 0 {
		return
	}
	seen := map[string]bool{}
	var keys []string
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var saved struct {
			Owner string `json:"owner"`
		}
		if json.Unmarshal(raw, &saved) == nil && saved.Owner != "" && !seen[saved.Owner] {
			seen[saved.Owner] = true
			keys = append(keys, saved.Owner)
		}
	}
	if len(keys) > 0 {
		c.o.key = strings.Join(keys, ",")
	}
}

func (c *cliClient) readBodyFlag() (string, bool, error) {
	if c.o.fileFlag != "" && c.o.bodySet {
		return "", false, errors.New("give --body or -f, not both")
	}
	if c.o.fileFlag != "" {
		var raw []byte
		var err error
		if c.o.fileFlag == "-" {
			raw, err = io.ReadAll(io.LimitReader(c.stdin, 8<<20))
		} else {
			raw, err = os.ReadFile(c.o.fileFlag)
		}
		if err != nil {
			return "", false, err
		}
		return string(raw), true, nil
	}
	if c.o.bodySet {
		return c.o.body, true, nil
	}
	return "", false, nil
}

func (c *cliClient) callRaw(action string, payload map[string]any, projectFields bool) (map[string]any, int) {
	payload["action"] = action
	raw, _ := json.Marshal(payload)
	q := ""
	if projectFields && c.fields != "" {
		q = "?fields=" + url.QueryEscape(c.fields)
	}
	req, err := http.NewRequest("POST", c.base()+"/api/action"+q, bytes.NewReader(raw))
	if err != nil {
		return nil, kit.UsageError(c.out, tool, c.o.asJSON, err.Error())
	}
	if err := kit.SetPublicMount(req, c.o.publicURL); err != nil {
		return nil, kit.UsageError(c.out, tool, c.o.asJSON, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	if c.o.key != "" {
		req.Header.Set("X-Facilitator-Key", c.o.key)
	}
	if c.o.member != "" {
		req.Header.Set("X-Calendar-Member", c.o.member)
	}
	httpClient := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}
	res, err := httpClient.Do(req)
	if err != nil {
		msg := fmt.Sprintf("calendar server at %s is not reachable: %v", c.base(), err)
		return nil, emitFailure(c.out, c.o.asJSON, "unavailable", msg, 1)
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if res.StatusCode != http.StatusOK {
		if c.o.asJSON {
			fmt.Fprintln(c.out, strings.TrimSpace(string(buf)))
		} else {
			fmt.Fprintln(c.out, "error:", kit.ErrorMessage(buf))
		}
		if res.StatusCode == http.StatusBadRequest {
			return nil, 2
		}
		return nil, 1
	}
	var obj map[string]any
	if err := json.Unmarshal(buf, &obj); err != nil {
		return nil, emitFailure(c.out, c.o.asJSON, "invalid_response", "invalid server response", 1)
	}
	return obj, 0
}

func (c *cliClient) doAction(action string, payload map[string]any) int {
	obj, code := c.callRaw(action, payload, true)
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
		for _, raw := range items {
			if m, ok := raw.(map[string]any); ok {
				fmt.Fprintf(c.out, "%v  %v  %v -> %v (%v)  [%v]  %v\n",
					m["date"], m["id"], m["display_start"], m["display_end"], m["display_tz"], m["status"], m["title"])
			}
		}
	case "show":
		fmt.Fprintf(c.out, "%v (%v) [%v]\n%v -> %v (%v)\n",
			obj["title"], obj["id"], obj["status"], obj["start"], obj["end"], obj["tz"])
		if loc, _ := obj["location"].(string); loc != "" {
			fmt.Fprintf(c.out, "location: %s\n", loc)
		}
		if body, _ := obj["body"].(string); body != "" {
			fmt.Fprintf(c.out, "\n%s\n", body)
		}
	default:
		// add/open/set/cancel/close/share: id, URL, status only — never content.
		fmt.Fprintf(c.out, "%v %v %v\n", obj["id"], obj["url"], obj["status"])
		if inv, ok := obj["inviteURL"]; ok {
			fmt.Fprintf(c.out, "invite: %v\n", inv)
		} else if inv, ok := obj["invite"]; ok {
			fmt.Fprintf(c.out, "invite: %v\n", inv)
		}
		if k, ok := obj["ownerKey"]; ok {
			fmt.Fprintf(c.out, "ownerKey: %v (keep private)\n", k)
		}
	}
	return 0
}

func (c *cliClient) cmdAdd() int {
	id := c.o.idFlag
	if id == "" && len(c.o.args) > 0 {
		id = c.o.args[0]
	}
	body, hasBody, err := c.readBodyFlag()
	if err != nil {
		return kit.UsageError(c.out, tool, c.o.asJSON, err.Error())
	}
	payload := map[string]any{}
	if id != "" {
		payload["id"] = id
	}
	if c.o.titleSet {
		payload["title"] = c.o.title
	}
	if c.o.startSet {
		payload["start"] = c.o.start
	}
	if c.o.endSet {
		payload["end"] = c.o.end
	}
	if c.o.tzSet {
		payload["tz"] = c.o.tz
	}
	if c.o.locationSet {
		payload["location"] = c.o.location
	}
	if hasBody {
		payload["body"] = body
	}
	if c.o.refsSet {
		payload["refs"] = c.o.refs
	}
	if c.o.audienceSet {
		payload["audience"] = c.o.audience
	}
	if c.dryRun {
		payload["dry_run"] = true
	}
	return c.doAction("add", payload)
}

func (c *cliClient) cmdSet() int {
	id, ok := c.needID()
	if !ok {
		return 2
	}
	c.resolveKey(id)
	body, hasBody, err := c.readBodyFlag()
	if err != nil {
		return kit.UsageError(c.out, tool, c.o.asJSON, err.Error())
	}
	payload := map[string]any{"id": id}
	if c.o.titleSet {
		payload["title"] = c.o.title
	}
	if c.o.startSet {
		payload["start"] = c.o.start
	}
	if c.o.endSet {
		payload["end"] = c.o.end
	}
	if c.o.tzSet {
		payload["tz"] = c.o.tz
	}
	if c.o.locationSet {
		payload["location"] = c.o.location
	}
	if hasBody {
		payload["body"] = body
	}
	if c.o.refsSet {
		payload["refs"] = c.o.refs
	}
	if c.o.audienceSet {
		payload["audience"] = c.o.audience
	}
	if c.o.statusSet {
		payload["status"] = c.o.status
	}
	if c.o.revoke != "" {
		payload["revoke"] = c.o.revoke
	}
	if c.dryRun {
		payload["dry_run"] = true
	}
	return c.doAction("set", payload)
}

func (c *cliClient) cmdList() int {
	c.resolveAllKeys()
	payload := map[string]any{}
	if c.o.from != "" {
		payload["from"] = c.o.from
	}
	if c.o.to != "" {
		payload["to"] = c.o.to
	}
	if c.o.tzSet {
		payload["tz"] = c.o.tz
	} else if envTZ := strings.TrimSpace(os.Getenv("TZ")); envTZ != "" {
		if _, err := validateTZ(envTZ); err == nil {
			payload["tz"] = envTZ
		}
	}
	if c.o.audienceSet {
		payload["audience"] = c.o.audience
	}
	if c.o.statusSet {
		payload["status"] = c.o.status
	}
	return c.doAction("list", payload)
}

func (c *cliClient) cmdJoin() int {
	if len(c.o.args) < 2 {
		return kit.UsageError(c.out, tool, c.o.asJSON, "calendar join ID INVITE")
	}
	id, invite := c.o.args[0], c.o.args[1]
	if !kit.ValidName(id) {
		return kit.UsageError(c.out, tool, c.o.asJSON, "use 1-64 letters, digits, - or _ for an id")
	}
	req := &http.Request{Header: make(http.Header)}
	if err := kit.SetPublicMount(req, c.o.publicURL); err != nil {
		return kit.UsageError(c.out, tool, c.o.asJSON, err.Error())
	}
	base := c.base()
	if c.o.publicURL != "" {
		base = strings.TrimRight(c.o.publicURL, "/") + c.o.prefix
	}
	joinURL := base + "/?id=" + url.QueryEscape(id) + "#invite=" + invite
	if c.o.asJSON {
		return emitLocalEnvelope(c.out, true, c.fields, map[string]any{
			"id":        id,
			"inviteURL": joinURL,
		})
	}
	fmt.Fprintln(c.out, joinURL)
	return 0
}

// ---- Export & Import (ICS, JSON, Google) -----------------------------------

func (c *cliClient) cmdExport() int {
	id := c.o.idFlag
	if id == "" && len(c.o.args) > 0 {
		id = c.o.args[0]
	}
	if id != "" {
		if !kit.ValidName(id) {
			return kit.UsageError(c.out, tool, c.o.asJSON, "use 1-64 letters, digits, - or _ for an id")
		}
		c.resolveKey(id)
	} else {
		c.resolveAllKeys()
	}
	if id == "" && !c.o.all {
		return kit.UsageError(c.out, tool, c.o.asJSON, "specify an event ID or --all")
	}

	to := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(c.o.to), "."))
	if to == "" {
		return kit.UsageError(c.out, tool, c.o.asJSON, "specify export target with --to ics|json|google")
	}
	if to == "google" {
		return c.exportGoogle(id, c.o.all)
	}
	if to != "ics" && to != "ical" && to != "json" {
		return kit.UsageError(c.out, tool, c.o.asJSON, fmt.Sprintf("unsupported --to %q (supported: ics, json, google)", c.o.to))
	}

	outPath := c.o.output
	if outPath != "" && outPath != "-" {
		if abs, err := filepath.Abs(outPath); err == nil {
			outPath = abs
		}
	}
	payload := map[string]any{
		"id":      id,
		"all":     c.o.all,
		"to":      to,
		"output":  outPath,
		"dry_run": c.dryRun,
	}
	obj, code := c.callRaw("export", payload, true)
	if code != 0 {
		return code
	}
	if c.o.asJSON {
		fmt.Fprintln(c.out, kit.CanonicalJSON(obj))
		return 0
	}
	if outPath == "-" {
		fmt.Fprint(c.out, obj["content"])
		return 0
	}
	fmt.Fprintln(c.out, obj["path"])
	return 0
}

func (c *cliClient) cmdImport() int {
	c.resolveAllKeys()
	from := strings.ToLower(strings.TrimSpace(c.o.from))
	if from == "" {
		from = "ics"
	}
	switch from {
	case "ics", "ical":
		if c.o.fileFlag == "" {
			return kit.UsageError(c.out, tool, c.o.asJSON, "import --from ics requires -f FILE|-")
		}
		var raw []byte
		var err error
		if c.o.fileFlag == "-" {
			raw, err = io.ReadAll(io.LimitReader(c.stdin, 8<<20))
		} else {
			raw, err = os.ReadFile(c.o.fileFlag)
		}
		if err != nil {
			return emitFailure(c.out, c.o.asJSON, "read_failed", err.Error(), 1)
		}
		obj, code := c.callRaw("import", map[string]any{
			"ics":     string(raw),
			"dry_run": c.dryRun,
		}, true)
		if code != 0 {
			return code
		}
		if c.o.asJSON {
			fmt.Fprintln(c.out, kit.CanonicalJSON(obj))
		} else {
			fmt.Fprintf(c.out, "imported %v event(s) (created=%v updated=%v)\n", obj["count"], obj["created"], obj["updated"])
		}
		return 0

	case "google":
		return c.importGoogle()

	default:
		return kit.UsageError(c.out, tool, c.o.asJSON, fmt.Sprintf("unsupported --from %q (supported: ics, google)", c.o.from))
	}
}

// ---- Google Calendar Interop via BASHY_GOOGLE_CMD --------------------------

func resolveGoogleCmd() ([]string, error) {
	raw := strings.TrimSpace(os.Getenv("BASHY_GOOGLE_CMD"))
	if raw != "" {
		parts := splitCommandLine(raw)
		if len(parts) == 0 {
			return nil, errors.New("BASHY_GOOGLE_CMD is empty")
		}
		if _, err := exec.LookPath(parts[0]); err != nil {
			return nil, fmt.Errorf("CLI_NOT_FOUND: google command %q not found (%v); register examples/google (./examples/google/register.sh) and onboard BYOP credentials with 'bashy google auth-guide'", parts[0], err)
		}
		return parts, nil
	}
	bin := strings.TrimSpace(os.Getenv("BASHY_BIN"))
	if bin == "" {
		bin = "bashy"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("CLI_NOT_FOUND: %q not found on PATH; install bashy, run ./examples/google/register.sh, and onboard BYOP credentials with 'bashy google auth-guide'", bin)
	}
	return []string{bin, "google"}, nil
}

func splitCommandLine(s string) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				cur.WriteByte(ch)
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == ' ' || ch == '\t' {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func invokeGoogleCLI(subArgs ...string) ([]byte, string, error) {
	baseCmd, err := resolveGoogleCmd()
	if err != nil {
		return nil, "CLI_NOT_FOUND", err
	}
	argv := append(append([]string{}, baseCmd...), subArgs...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), "BASHY_HINTS=off")
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr := cmd.Run()
	if runErr == nil {
		return stdoutBuf.Bytes(), "", nil
	}

	combined := strings.TrimSpace(stderrBuf.String())
	if combined == "" {
		combined = strings.TrimSpace(stdoutBuf.String())
	}
	// Extract structured error code from google.bsh if present.
	var gErr struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(combined), &gErr) == nil && gErr.Error.Code != "" {
		code := gErr.Error.Code
		if code == "GWS_NOT_FOUND" {
			code = "CLI_NOT_FOUND"
		}
		return nil, code, fmt.Errorf("%s: %s (onboard BYOP credentials with 'bashy google auth-guide' after ./examples/google/register.sh)", code, gErr.Error.Message)
	}
	lower := strings.ToLower(combined + " " + runErr.Error())
	if strings.Contains(lower, "not found") || strings.Contains(lower, "unknown command") || strings.Contains(lower, "no such file") {
		return nil, "CLI_NOT_FOUND", fmt.Errorf("CLI_NOT_FOUND: google CLI is not registered or missing (%s); run ./examples/google/register.sh and 'bashy google auth-guide'", combined)
	}
	if strings.Contains(lower, "credential") || strings.Contains(lower, "account") || strings.Contains(lower, "auth") {
		return nil, "CLI_NOT_FOUND", fmt.Errorf("CLI_NOT_FOUND: Google BYOP credentials are not configured (%s); run 'bashy google auth-guide'", combined)
	}
	return nil, "google_failed", fmt.Errorf("google %s failed: %s", subArgs[0], combined)
}

type googleEventTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
	TimeZone string `json:"timeZone"`
}

type googleEventItem struct {
	ID          string          `json:"id"`
	Summary     string          `json:"summary"`
	Description string          `json:"description"`
	Location    string          `json:"location"`
	Status      string          `json:"status"`
	HTMLLink    string          `json:"htmlLink"`
	Start       googleEventTime `json:"start"`
	End         googleEventTime `json:"end"`
}

func parseGoogleEventTime(gt googleEventTime, fallbackTZ string, isEnd bool) (string, string, error) {
	tz := strings.TrimSpace(gt.TimeZone)
	if tz != "" {
		if _, err := validateTZ(tz); err != nil {
			return "", "", err
		}
	} else if fallbackTZ != "" {
		tz = fallbackTZ
	} else {
		tz = "UTC"
	}
	loc, err := validateTZ(tz)
	if err != nil {
		return "", "", err
	}
	if gt.DateTime != "" {
		t, pErr := time.Parse(time.RFC3339, gt.DateTime)
		if pErr != nil {
			return "", "", fmt.Errorf("invalid Google dateTime %q: %v", gt.DateTime, pErr)
		}
		return t.In(loc).Format(time.RFC3339), tz, nil
	}
	if gt.Date != "" {
		d, pErr := time.ParseInLocation("2006-01-02", gt.Date, loc)
		if pErr != nil {
			return "", "", fmt.Errorf("invalid Google date %q: %v", gt.Date, pErr)
		}
		if isEnd && d.IsZero() {
			return "", "", errors.New("invalid Google date")
		}
		return d.Format(time.RFC3339), tz, nil
	}
	return "", "", errors.New("Google event missing start/end dateTime or date")
}

func (c *cliClient) importGoogle() int {
	calID := c.o.calendarID
	if calID == "" {
		calID = "primary"
	}
	out, errCode, err := invokeGoogleCLI("calendar-list", "calendarId="+calID)
	if err != nil {
		return emitFailure(c.out, c.o.asJSON, errCode, err.Error(), 1)
	}
	var listResp struct {
		Items []googleEventItem `json:"items"`
	}
	if err := json.Unmarshal(out, &listResp); err != nil {
		return emitFailure(c.out, c.o.asJSON, "invalid_google_json", fmt.Sprintf("invalid calendar-list JSON: %v", err), 1)
	}

	specs := make([]upsertSpec, 0, len(listResp.Items))
	for _, gi := range listResp.Items {
		if strings.TrimSpace(gi.ID) == "" {
			continue
		}
		startStr, tz, err := parseGoogleEventTime(gi.Start, "", false)
		if err != nil {
			return emitFailure(c.out, c.o.asJSON, "invalid_google_event", err.Error(), 1)
		}
		endStr := ""
		if gi.End.DateTime == "" && gi.End.Date == "" {
			st, _ := time.Parse(time.RFC3339, startStr)
			loc, _ := validateTZ(tz)
			endStr = st.Add(time.Hour).In(loc).Format(time.RFC3339)
		} else {
			endStr, _, err = parseGoogleEventTime(gi.End, tz, true)
			if err != nil {
				return emitFailure(c.out, c.o.asJSON, "invalid_google_event", err.Error(), 1)
			}
		}
		status := kit.StatusPending
		if strings.EqualFold(gi.Status, "cancelled") {
			status = StatusCancelled
		}
		var refs []string
		if strings.TrimSpace(gi.HTMLLink) != "" {
			refs = []string{strings.TrimSpace(gi.HTMLLink)}
		}
		title := strings.TrimSpace(gi.Summary)
		if title == "" {
			title = "Untitled event"
		}
		specs = append(specs, upsertSpec{
			Title:    title,
			Start:    startStr,
			End:      endStr,
			TZ:       tz,
			Location: gi.Location,
			Body:     gi.Description,
			Refs:     refs,
			Status:   status,
			Source:   eventSource{Provider: "google", UID: gi.ID},
		})
	}

	obj, code := c.callRaw("import", map[string]any{
		"from":    "google",
		"events":  specs,
		"dry_run": c.dryRun,
	}, true)
	if code != 0 {
		return code
	}
	if c.o.asJSON {
		fmt.Fprintln(c.out, kit.CanonicalJSON(obj))
	} else {
		fmt.Fprintf(c.out, "imported %v Google event(s) (created=%v updated=%v)\n", obj["count"], obj["created"], obj["updated"])
	}
	return 0
}

func (c *cliClient) fetchFullEvents(id string, all bool) ([]portableEvent, int) {
	if id != "" {
		obj, code := c.callRaw("show", map[string]any{"id": id}, false)
		if code != 0 {
			return nil, code
		}
		b, _ := json.Marshal(obj)
		var pe portableEvent
		_ = json.Unmarshal(b, &pe)
		return []portableEvent{pe}, 0
	}
	listObj, code := c.callRaw("list", map[string]any{}, false)
	if code != 0 {
		return nil, code
	}
	rawItems, _ := listObj["items"].([]any)
	var out []portableEvent
	for _, it := range rawItems {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		eid, _ := m["id"].(string)
		if eid == "" {
			continue
		}
		showObj, sc := c.callRaw("show", map[string]any{"id": eid}, false)
		if sc != 0 {
			return nil, sc
		}
		b, _ := json.Marshal(showObj)
		var pe portableEvent
		_ = json.Unmarshal(b, &pe)
		out = append(out, pe)
	}
	return out, 0
}

func (c *cliClient) exportGoogle(id string, all bool) int {
	calID := c.o.calendarID
	if calID == "" {
		calID = "primary"
	}
	// Verify Google CLI availability upfront (including under --dry-run).
	if _, err := resolveGoogleCmd(); err != nil {
		return emitFailure(c.out, c.o.asJSON, "CLI_NOT_FOUND", err.Error(), 1)
	}

	evs, code := c.fetchFullEvents(id, all)
	if code != 0 {
		return code
	}

	if c.dryRun {
		plans := make([]map[string]any, 0, len(evs))
		for _, ev := range evs {
			op := "calendar-create"
			if ev.Source.Provider == "google" && ev.Source.UID != "" {
				op = "calendar-update"
			}
			plans = append(plans, map[string]any{
				"id":        ev.ID,
				"operation": op,
				"calendar":  calID,
				"uid":       ev.Source.UID,
			})
		}
		return emitLocalEnvelope(c.out, true, c.fields, map[string]any{
			"verb":     "export",
			"format":   "google",
			"calendar": calID,
			"dry_run":  true,
			"count":    len(plans),
			"items":    plans,
		})
	}

	createdN, updatedN := 0, 0
	results := make([]map[string]any, 0, len(evs))
	for _, ev := range evs {
		kvArgs := []string{
			"calendarId=" + calID,
			"summary=" + ev.Title,
			"start=" + ev.Start,
			"end=" + ev.End,
		}
		if ev.Location != "" {
			kvArgs = append(kvArgs, "location="+ev.Location)
		}
		if ev.Body != "" {
			kvArgs = append(kvArgs, "description="+ev.Body)
		}

		if ev.Source.Provider == "google" && ev.Source.UID != "" {
			callArgs := append([]string{"calendar-update", ev.Source.UID}, kvArgs...)
			out, errCode, err := invokeGoogleCLI(callArgs...)
			if err != nil {
				return emitFailure(c.out, c.o.asJSON, errCode, err.Error(), 1)
			}
			var resp struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(out, &resp)
			gid := resp.ID
			if gid == "" {
				gid = ev.Source.UID
			}
			updatedN++
			results = append(results, map[string]any{
				"id":        ev.ID,
				"operation": "updated",
				"google_id": gid,
			})
			continue
		}

		callArgs := append([]string{"calendar-create"}, kvArgs...)
		out, errCode, err := invokeGoogleCLI(callArgs...)
		if err != nil {
			return emitFailure(c.out, c.o.asJSON, errCode, err.Error(), 1)
		}
		var resp struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(out, &resp); err != nil || strings.TrimSpace(resp.ID) == "" {
			return emitFailure(c.out, c.o.asJSON, "invalid_google_json", "calendar-create did not return an event id", 1)
		}
		// Record Google's event id on the local event so re-export updates instead of duplicating.
		c.resolveKey(ev.ID)
		_, sc := c.callRaw("set", map[string]any{
			"id":     ev.ID,
			"source": map[string]any{"provider": "google", "uid": resp.ID},
		}, false)
		if sc != 0 {
			return sc
		}
		createdN++
		results = append(results, map[string]any{
			"id":        ev.ID,
			"operation": "created",
			"google_id": resp.ID,
		})
	}

	if c.o.asJSON {
		return emitLocalEnvelope(c.out, true, c.fields, map[string]any{
			"verb":     "export",
			"format":   "google",
			"calendar": calID,
			"count":    len(results),
			"created":  createdN,
			"updated":  updatedN,
			"items":    results,
		})
	}
	fmt.Fprintf(c.out, "exported %d event(s) to Google Calendar (%s): created=%d updated=%d\n",
		len(results), calID, createdN, updatedN)
	return 0
}

// ---- Git-Directory save / load (never invokes git) -------------------------

func (c *cliClient) targetGitDir() string {
	dir := c.o.gitDir
	if dir == "" && len(c.o.args) > 0 {
		dir = c.o.args[0]
	}
	if dir == "" {
		dir = filepath.Join("docs", "calendar")
	}
	return filepath.Clean(dir)
}

func (c *cliClient) cmdSave() int {
	c.resolveAllKeys()
	dir := c.targetGitDir()
	evs, code := c.fetchFullEvents("", true)
	if code != 0 {
		return code
	}
	sort.Slice(evs, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, evs[i].Start)
		tj, _ := time.Parse(time.RFC3339, evs[j].Start)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return evs[i].ID < evs[j].ID
	})

	files := []string{filepath.Join(dir, "events.json")}
	for _, pe := range evs {
		files = append(files, filepath.Join(dir, pe.ID+".ics"))
	}

	if c.dryRun {
		return emitLocalEnvelope(c.out, true, c.fields, map[string]any{
			"verb":    "save",
			"dir":     dir,
			"count":   len(evs),
			"files":   files,
			"dry_run": true,
		})
	}

	eventsBytes, err := kit.MarshalIndent(map[string]any{
		"schema_version": kit.SchemaVersion(tool),
		"items":          evs,
	})
	if err != nil {
		return emitFailure(c.out, c.o.asJSON, "marshal_failed", err.Error(), 1)
	}
	if err := writeRegularAtomic(filepath.Join(dir, "events.json"), append(eventsBytes, '\n')); err != nil {
		return emitFailure(c.out, c.o.asJSON, "write_failed", err.Error(), 1)
	}

	for _, pe := range evs {
		ev := &event{
			ID: pe.ID, Title: pe.Title, Start: pe.Start, End: pe.End, TZ: pe.TZ,
			Location: pe.Location, Body: pe.Body, Refs: pe.Refs, Audience: pe.Audience,
			Status: pe.Status, Source: pe.Source, Created: pe.Created, Updated: pe.Updated,
		}
		icsBytes := FormatICS([]*event{ev})
		if err := writeRegularAtomic(filepath.Join(dir, pe.ID+".ics"), icsBytes); err != nil {
			return emitFailure(c.out, c.o.asJSON, "write_failed", err.Error(), 1)
		}
	}

	if c.o.asJSON {
		return emitLocalEnvelope(c.out, true, c.fields, map[string]any{
			"verb":  "save",
			"dir":   dir,
			"count": len(evs),
			"files": files,
		})
	}
	fmt.Fprintf(c.out, "saved %d event(s) to %s\n", len(evs), dir)
	return 0
}

func (c *cliClient) cmdLoad() int {
	c.resolveAllKeys()
	dir := c.targetGitDir()
	eventsFile := filepath.Join(dir, "events.json")

	var specs []upsertSpec
	if raw, err := os.ReadFile(eventsFile); err == nil {
		var wrapped struct {
			Items []upsertSpec `json:"items"`
		}
		if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Items != nil {
			specs = wrapped.Items
		} else {
			var arr []upsertSpec
			if err2 := json.Unmarshal(raw, &arr); err2 != nil {
				return emitFailure(c.out, c.o.asJSON, "invalid_json", fmt.Sprintf("cannot parse %s: %v", eventsFile, err), 1)
			}
			specs = arr
		}
	} else {
		icsFiles, gErr := filepath.Glob(filepath.Join(dir, "*.ics"))
		if gErr != nil || len(icsFiles) == 0 {
			return emitFailure(c.out, c.o.asJSON, "not_found", fmt.Sprintf("no events.json or *.ics found in %s", dir), 1)
		}
		sort.Strings(icsFiles)
		for _, f := range icsFiles {
			rawICS, rErr := os.ReadFile(f)
			if rErr != nil {
				return emitFailure(c.out, c.o.asJSON, "read_failed", rErr.Error(), 1)
			}
			parsed, pErr := ParseICS(rawICS)
			if pErr != nil {
				return emitFailure(c.out, c.o.asJSON, "invalid_ics", fmt.Sprintf("%s: %v", f, pErr), 1)
			}
			baseID := strings.TrimSuffix(filepath.Base(f), ".ics")
			for i := range parsed {
				if parsed[i].ID == "" && kit.ValidName(baseID) {
					parsed[i].ID = baseID
				}
			}
			specs = append(specs, parsed...)
		}
	}

	obj, code := c.callRaw("load", map[string]any{
		"events":  specs,
		"dry_run": c.dryRun,
	}, true)
	if code != 0 {
		return code
	}
	if c.o.asJSON {
		fmt.Fprintln(c.out, kit.CanonicalJSON(obj))
	} else {
		fmt.Fprintf(c.out, "loaded %v event(s) from %s (created=%v updated=%v)\n",
			obj["count"], dir, obj["created"], obj["updated"])
	}
	return 0
}

// ---- Envelope Helpers ------------------------------------------------------

func emitLocalEnvelope(out io.Writer, asJSON bool, fields string, body map[string]any) int {
	env := kit.Envelope(tool, body)
	projected, err := kit.Project(env, fields)
	if err != nil {
		return kit.UsageError(out, tool, asJSON, err.Error())
	}
	fmt.Fprintln(out, kit.CanonicalJSON(projected))
	return 0
}

func emitFailure(out io.Writer, asJSON bool, code, message string, exit int) int {
	if asJSON {
		fmt.Fprintln(out, kit.CanonicalJSON(kit.Envelope(tool, map[string]any{
			"error": map[string]any{"code": code, "message": message},
		})))
	} else {
		fmt.Fprintln(out, "error:", message)
	}
	return exit
}
