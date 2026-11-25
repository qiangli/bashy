// The form agent door: add|ask|open|show|set|list|share|close|wait over the
// same JSON API the page uses. --json prints the raw API object; without
// it, open/new print only id, URL and status — never content. wait polls
// get until the human submits (answered) or the form closes.
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
	"path/filepath"
	"strings"
	"time"

	kit "github.com/qiangli/bashy/examples/desk/kit"
)

const defaultFormAddr = "127.0.0.1:18107"

const formUsage = `Ad-hoc loopback forms: define in JSON/YAML, human fills on the page, agent reads answers as JSON.

Usage:
  form VERB [ID] [flags]

Commands:
  serve               Start loopback form server (default 127.0.0.1:18107; custom -data uses its own port)
  add [FILE|-]        Create a form from --def FILE|- (or positional FILE), --definition, or --poll
  ask [ID|FILE]       Create a Q1..Qn questionnaire, or append a follow-up round to ID (with --key)
  reply               Create a web-form reply checklist from --extract FILE|- [--values FILE] [--refusals FILE]
  open ID             Print id, loopback URL, and status only (never content)
  show ID             Read definition and answers (owner --key/FORM_KEY or joined participant)
  wait ID             Poll until answered, closed, or --timeout expires (default 5m; resolves local owner key)
  set ID              Update title/body/definition or prefill answers (--answer k=v, repeatable)
  answer ID           Submit respondent answers (--answer k=v or --answers FILE|- [--complete])
  list                List forms in the store (id, status, URL)
  share ID            Mint a single-use invite link (or reusable URL when --respondents many)
  join ID [INVITE]    Redeem a single-use --invite CODE
  close ID            Close a form against further responses

Flags:
  --def FILE|-            Form definition file in JSON or YAML (- for stdin)
  --definition TEXT       Inline JSON or YAML form definition
  --title TEXT            Form title
  --body TEXT             Agent note (returned by show, hidden by open/list)
  --id ID                 Explicit form id on add/ask/reply
  --key KEY               Owner capability key (or FORM_KEY; wait resolves local owner key automatically)
  --timeout DURATION      wait: give up after this long (default 5m, e.g. 30m, 4h)
  --interval DURATION     wait: poll interval (default 2s)
  --answer KEY=VALUE      Field answer pair (repeatable on set and answer)
  --answers FILE|-        Respondent answers as JSON object (- for stdin)
  --complete              answer: submit complete respondent answer (checks required fields)
  --poll QUESTION         add: one-question multi-respondent poll (requires --options)
  --options A,B,C         add: comma-separated choices for --poll
  --respondents MODE      one (default) or many
  --anonymous             Do not ask participants for a name
  --results-visible       Show live aggregate results to respondents
  --close-at RFC3339      Automatic close timestamp
  --extract FILE|-        reply: browser extract JSON
  --values FILE           reply: proposed values JSON
  --refusals FILE         reply: guard refusals/failures JSON
  --invite CODE           join: single-use invite token
  --participant TOKEN     Respondent session token
  --addr HOST:PORT        Loopback server address (or FORM_ADDR; default 127.0.0.1:18107)
  --data DIR              Form store directory (or FORM_DATA; default $BASHY_HOME/desk/form)
  --prefix /PATH          URL path prefix (or FORM_PREFIX)
  --json                  Print raw versioned JSON response
  --fields F1,F2          Project top-level JSON fields (requires --json)
  --dry-run               Preview action without mutating state
  --explain               Describe the action as versioned JSON
  --no-input              Never prompt for input
  --help=mcp              Print MCP tool metadata JSON
  -h, --help              Show this help`

// answerFlags collects repeated --answer key=value pairs.
type answerFlags map[string]string

func (a answerFlags) String() string { return fmt.Sprint(map[string]string(a)) }

func (a answerFlags) Set(s string) error {
	k, v, ok := strings.Cut(s, "=")
	if !ok || k == "" {
		return fmt.Errorf("want --answer key=value, got %q", s)
	}
	a[k] = v
	return nil
}

func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "-h" || a == "--help" || a == "-help" {
			return true
		}
	}
	return false
}

func runCLI(args []string, stdout io.Writer) int {
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, formUsage)
		return 0
	}
	var common kit.CommonOptions
	var commonErr error
	args, common, commonErr = kit.ParseCommon(args)
	if commonErr != nil {
		fmt.Fprintln(stdout, "error:", commonErr)
		return 2
	}
	if common.HelpMCP {
		help := kit.MCPHelp("form")
		fmt.Fprintln(stdout, kit.CanonicalJSON(help))
		return 0
	}
	if common.Explain || common.DryRun {
		verb := kit.FindVerb("form", args)
		if verb == "" {
			fmt.Fprintln(stdout, "error: unknown verb")
			return 2
		}
		fmt.Fprintln(stdout, kit.CanonicalJSON(kit.Envelope("form", map[string]any{"verb": verb, "effects": "desk action", "dry_run": common.DryRun})))
		return 0
	}

	defaultAddr := defaultFormAddr
	defaultData := kit.DefaultDataDir("form")
	defaultPrefix := os.Getenv("FORM_PREFIX")
	publicURL := os.Getenv("FORM_PUBLIC_URL")
	explicitAddr := false
	explicitData := false
	if os.Getenv("BASHY_APP") == "form" {
		if v := os.Getenv("BASHY_APP_ADDR"); v != "" {
			defaultAddr = v
			explicitAddr = true
		}
		if v := os.Getenv("BASHY_APP_DATA"); v != "" {
			defaultData = v
			explicitData = true
		}
	}
	if v := os.Getenv("FORM_ADDR"); v != "" {
		defaultAddr = v
		explicitAddr = true
	}
	if v := os.Getenv("FORM_DATA"); v != "" {
		defaultData = v
		explicitData = true
	}

	// The verb comes first (form show ID --json); connection flags may
	// precede it (form -addr 127.0.0.1:18107 show ID). Split the argv at
	// the first non-flag token so flags work on either side.
	pre, verb, post := splitVerb(args)
	if verb == "" {
		fmt.Fprintln(stdout, "Usage: form [serve|add|reply|ask|open|show|set|list|share|close|wait|answer|join] [--json]")
		return 2
	}
	fs := flag.NewFlagSet("form "+verb, flag.ContinueOnError)
	addr := fs.String("addr", defaultAddr, "tool server address")
	data := fs.String("data", defaultData, "data directory")
	prefix := fs.String("prefix", defaultPrefix, "URL prefix when served behind a sub-path")
	asJSON := fs.Bool("json", common.JSON, "print the raw JSON API object")
	title := fs.String("title", "", "form title")
	body := fs.String("body", "", "form body (agent note, returned by show)")
	idFlag := fs.String("id", "", "form id (add)")
	extractPath := fs.String("extract", "", "browser extract JSON file (- for stdin)")
	valuesPath := fs.String("values", "", "proposals keyed by stable_id")
	refusalsPath := fs.String("refusals", "", "fill results keyed by stable_id")
	defPath := fs.String("def", "", "form definition file (JSON or YAML; - for stdin)")
	defInline := fs.String("definition", "", "form definition inline (JSON or YAML)")
	respondents := fs.String("respondents", "", "one or many participants")
	anonymous := fs.Bool("anonymous", false, "do not ask participants for a name")
	closeAt := fs.String("close-at", "", "optional RFC3339 close time")
	resultsVisible := fs.Bool("results-visible", false, "show aggregate results to respondents")
	poll := fs.String("poll", "", "create a one-question poll")
	options := fs.String("options", "", "comma-separated poll choices")
	key := fs.String("key", "", "owner key (FORM_KEY also works)")
	participant := fs.String("participant", "", "stable respondent token")
	invite := fs.String("invite", "", "single-use invite code for join")
	timeout := fs.Duration("timeout", 5*time.Minute, "wait: give up after this long")
	interval := fs.Duration("interval", 2*time.Second, "wait: poll interval")
	answers := answerFlags{}
	fs.Var(answers, "answer", "prefill answer as key=value (repeatable, with set/answer)")
	answersPath := fs.String("answers", "", "answer values as JSON file (- for stdin)")
	complete := fs.Bool("complete", false, "submit complete respondent answer")
	positional, visited, err := parseVerbArgsWithSet(fs, append(pre, post...))
	if err != nil {
		if *asJSON {
			return kit.UsageError(stdout, "form", true, err.Error())
		}
		fmt.Fprintf(stdout, "error: %v\n", err)
		return 2
	}
	if visited["addr"] {
		explicitAddr = true
	}
	if visited["data"] {
		explicitData = true
	}
	if *key == "" {
		*key = os.Getenv("FORM_KEY")
	}
	if verb == "serve" {
		serveWithOptions(*addr, *data, *prefix, explicitAddr)
		return 0
	}
	switch verb {
	case "answer", "reply", "ask", "add", "open", "show", "wait", "set", "list", "share", "close", "join":
	default:
		return kit.UsageError(stdout, "form", *asJSON, fmt.Sprintf("unknown verb %q", verb))
	}

	// A definition for add/ask/set comes from --def, --definition, or neither.
	loadDef := func() (any, int) {
		src, name := *defInline, "inline"
		if *defPath != "" {
			raw, err := readDefinitionBytes(*defPath)
			if err != nil {
				fmt.Fprintf(stdout, "error: %v\n", err)
				return nil, 1
			}
			src, name = string(raw), *defPath
		} else if src == "" {
			return nil, 0
		}
		t, def, mode, err := ParseFormDefinition([]byte(src), name)
		if err != nil {
			fmt.Fprintf(stdout, "error: %v\n", err)
			return nil, 1
		}
		if *title == "" {
			*title = t
		}
		if *respondents == "" {
			*respondents = mode
		}
		raw, _ := json.Marshal(def)
		var v any
		_ = json.Unmarshal(raw, &v)
		return v, 0
	}

	var (
		targetChecked bool
		targetStore   string
		lastRespStore string
	)
	failClient := func(codeStr, msg string, exitCode int) int {
		if *asJSON {
			fmt.Fprintln(stdout, kit.CanonicalJSON(kit.Envelope("form", map[string]any{
				"error": map[string]any{"code": codeStr, "message": msg},
			})))
		} else {
			fmt.Fprintf(stdout, "error: %s\n", msg)
		}
		return exitCode
	}
	ensureTarget := func() int {
		if targetChecked {
			return 0
		}
		targetChecked = true
		resolved := false
		if !explicitAddr {
			if info, ok := readServerInfo(*data); ok && info.Addr != "" {
				candPrefix := *prefix
				if candPrefix == "" {
					candPrefix = info.Prefix
				}
				if got, reachable := probeServerStore(info.Addr, candPrefix); reachable && got == storeID(*data) {
					*addr = info.Addr
					*prefix = candPrefix
					targetStore = got
					resolved = true
				}
			}
			if !resolved && !explicitData && os.Getenv("BASHY_HOME") == "" {
				if home, herr := os.UserHomeDir(); herr == nil && home != "" {
					altData := filepath.Join(home, ".bashy", "desk", "form")
					if info, ok := readServerInfo(altData); ok && info.Addr != "" {
						candPrefix := *prefix
						if candPrefix == "" {
							candPrefix = info.Prefix
						}
						if got, reachable := probeServerStore(info.Addr, candPrefix); reachable && got == storeID(altData) {
							*addr = info.Addr
							*prefix = candPrefix
							*data = altData
							targetStore = got
							resolved = true
						}
					}
					if !resolved {
						if got, reachable := probeServerStore(defaultFormAddr, *prefix); reachable && got == storeID(altData) {
							*addr = defaultFormAddr
							*data = altData
							targetStore = got
							resolved = true
						}
					}
				}
			}
			if !resolved && !explicitData {
				if got, reachable := probeServerStore(defaultFormAddr, *prefix); reachable && got == storeID(*data) {
					*addr = defaultFormAddr
					targetStore = got
					resolved = true
				} else if rt, derr := kit.DiscoverAppRuntime("form"); derr == nil {
					if gotRT, okRT := probeServerStore(rt.Addr, *prefix); okRT && gotRT == storeID(rt.Data) {
						*addr = rt.Addr
						*data = rt.Data
						if publicURL == "" {
							publicURL = rt.URL
						}
						targetStore = gotRT
						resolved = true
					}
				}
			}
		}
		if !resolved && (!explicitAddr || explicitData) {
			if got, reachable := probeServerStore(*addr, *prefix); reachable {
				want := storeID(*data)
				if got != want {
					return failClient(
						"store_mismatch",
						fmt.Sprintf("form server at http://%s%s serves a different form store (expected %s); start a server for this store (`form serve -data %s`) or pass matching --addr and --data", *addr, *prefix, *data, *data),
						1,
					)
				}
				targetStore = got
			}
		}
		return 0
	}

	call := func(action string, payload map[string]any) (map[string]any, []map[string]any, int) {
		if rc := ensureTarget(); rc != 0 {
			return nil, nil, rc
		}
		base := "http://" + *addr + *prefix
		payload["action"] = action
		raw, _ := json.Marshal(payload)
		req, err := http.NewRequest("POST", base+"/api/action"+commonQuery(common.Fields), bytes.NewReader(raw))
		if err != nil {
			return nil, nil, failClient("invalid_arg", err.Error(), 2)
		}
		if err := kit.SetPublicMount(req, publicURL); err != nil {
			return nil, nil, failClient("invalid_arg", err.Error(), 2)
		}
		req.Header.Set("Content-Type", "application/json")
		if *participant != "" {
			req.AddCookie(&http.Cookie{Name: "form-participant", Value: *participant})
		}
		if *key != "" {
			req.Header.Set("X-Facilitator-Key", *key)
		}
		client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}}
		r, err := client.Do(req)
		if err != nil {
			return nil, nil, failClient("unavailable", fmt.Sprintf("form server at %s is not reachable: %v", base, err), 1)
		}
		defer r.Body.Close()
		if s := r.Header.Get(storeHeader); s != "" {
			lastRespStore = s
		}
		buf, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if r.StatusCode != 200 {
			if *asJSON {
				fmt.Fprintln(stdout, strings.TrimSpace(string(buf)))
			} else {
				fmt.Fprintf(stdout, "error: %s\n", kit.ErrorMessage(buf))
			}
			if r.StatusCode == http.StatusBadRequest {
				return nil, nil, 2
			}
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
	rememberCreatedOwnerKey := func(obj map[string]any) {
		if obj == nil {
			return
		}
		id, _ := obj["id"].(string)
		ownerKey, _ := obj["ownerKey"].(string)
		if !kit.ValidName(id) || ownerKey == "" {
			return
		}
		scope := lastRespStore
		if scope == "" {
			scope = targetStore
		}
		saveClientOwnerKey(*data, serverKeyScope(scope, *addr, *prefix), id, ownerKey)
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
		case "show", "wait":
			fmt.Fprintf(stdout, "%v\n", obj["title"])
			if b, _ := obj["body"].(string); b != "" {
				fmt.Fprintf(stdout, "%v\n", b)
			}
			if ans, ok := obj["answers"].(map[string]any); ok {
				for _, k := range sortedKeys(ans) {
					fmt.Fprintf(stdout, "%s: %s\n", k, kit.CanonicalJSON(ans[k]))
				}
			}
			fmt.Fprintf(stdout, "status: %v\n", obj["status"])
		default:
			// add/open/set/share/close: id, URL, status only — never content.
			link := obj["url"]
			if inviteURL, ok := obj["inviteURL"].(string); ok && inviteURL != "" {
				link = inviteURL
			}
			fmt.Fprintf(stdout, "%v %v %v\n", obj["id"], link, obj["status"])
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
		if len(positional) == 0 {
			fmt.Fprintln(stdout, "error: give the form id")
			return "", false
		}
		if !kit.ValidName(positional[0]) {
			fmt.Fprintln(stdout, "error: use 1-64 letters, digits, - or _ for an id")
			return "", false
		}
		return positional[0], true
	}
	// Resolve the creating owner's key for wait when neither --key nor FORM_KEY
	// was supplied: first from the matching local item store, then from the
	// client's scoped owner-key cache written when this client created the form.
	resolveWaitKey := func(id string) int {
		if rc := ensureTarget(); rc != 0 {
			return rc
		}
		if *key != "" {
			return 0
		}
		scope := targetStore
		if scope == "" {
			if got, ok := probeServerStore(*addr, *prefix); ok {
				scope = got
				targetStore = got
			}
		}
		if !explicitAddr || scope == "" || scope == storeID(*data) {
			if k := readSavedOwner(itemPath(*data, id)); k != "" {
				*key = k
				return 0
			}
		}
		if k := readSavedOwner(clientKeyPath(*data, serverKeyScope(scope, *addr, *prefix), id)); k != "" {
			*key = k
		}
		return 0
	}
	switch verb {
	case "answer":
		id, ok := needID()
		if !ok {
			return 2
		}
		var values any = map[string]string(answers)
		if *answersPath != "" {
			raw, err := readDefinitionBytes(*answersPath)
			if err != nil || json.Unmarshal(raw, &values) != nil {
				fmt.Fprintln(stdout, "error: invalid answers JSON input")
				return 2
			}
		}
		obj, _, code := call("answer", map[string]any{"id": id, "answers": values, "complete": *complete})
		if code != 0 {
			return code
		}
		return emit(obj, nil)
	case "reply":
		if *extractPath == "" {
			fmt.Fprintln(stdout, "error: reply requires --extract FILE|-")
			return 2
		}
		payload := map[string]any{"id": *idFlag, "title": *title}
		for _, input := range []struct{ name, path string }{{"extract", *extractPath}, {"values", *valuesPath}, {"refusals", *refusalsPath}} {
			if input.path == "" {
				continue
			}
			raw, err := readDefinitionBytes(input.path)
			if err != nil || !json.Valid(raw) {
				fmt.Fprintf(stdout, "error: invalid %s JSON input\n", input.name)
				return 1
			}
			payload[input.name] = json.RawMessage(raw)
		}
		obj, _, code := call("reply", payload)
		if code != 0 {
			return code
		}
		rememberCreatedOwnerKey(obj)
		return emit(obj, nil)
	case "ask":
		if *defPath == "" && *defInline == "" && len(positional) == 1 && looksLikeDefArg(positional[0]) {
			*defPath = positional[0]
			positional = nil
		}
		def, code := loadDef()
		if code != 0 {
			return code
		}
		if def == nil {
			fmt.Fprintln(stdout, "error: form ask needs --def or --definition")
			return 2
		}
		payload := map[string]any{"id": *idFlag, "title": *title, "body": *body, "definition": def,
			"respondents": *respondents, "anonymous": *anonymous, "close_at": *closeAt, "results_visible": *resultsVisible}
		if len(positional) > 0 {
			payload["id"] = positional[0]
		} else {
			payload["create"] = true
		}
		obj, _, code := call("ask", payload)
		if code != 0 {
			return code
		}
		rememberCreatedOwnerKey(obj)
		return emit(obj, nil)
	case "add":
		if *defPath == "" && *defInline == "" && *poll == "" && len(positional) > 0 {
			if len(positional) > 1 {
				fmt.Fprintln(stdout, "error: form add accepts at most one definition file")
				return 2
			}
			*defPath = positional[0]
		}
		def, code := loadDef()
		if code != 0 {
			return code
		}
		if *poll != "" {
			if def != nil || strings.TrimSpace(*options) == "" {
				fmt.Fprintln(stdout, "error: --poll needs --options and cannot use --def")
				return 2
			}
			choices := []string{}
			for _, opt := range strings.Split(*options, ",") {
				opt = strings.TrimSpace(opt)
				if opt == "" {
					fmt.Fprintln(stdout, "error: poll options must be nonempty")
					return 2
				}
				choices = append(choices, opt)
			}
			def = Definition{{ID: "vote", Label: *poll, Kind: KindChoice, Required: true, Options: choices}}
			if *title == "" {
				*title = *poll
			}
			*respondents = "many"
		}
		payload := map[string]any{"id": *idFlag, "title": *title, "body": *body,
			"respondents": *respondents, "anonymous": *anonymous, "close_at": *closeAt, "results_visible": *resultsVisible}
		if def != nil {
			payload["definition"] = def
		}
		obj, _, code := call("add", payload)
		if code != 0 {
			return code
		}
		rememberCreatedOwnerKey(obj)
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
	case "wait":
		id, ok := needID()
		if !ok {
			return 2
		}
		if rc := resolveWaitKey(id); rc != 0 {
			return rc
		}
		deadline := time.Now().Add(*timeout)
		for {
			obj, _, code := call("show", map[string]any{"id": id})
			if code != 0 {
				return code
			}
			switch obj["status"] {
			case kit.StatusAnswered:
				return emit(obj, nil)
			case kit.StatusClosed, kit.StatusFailed:
				fmt.Fprintf(stdout, "error: form %s is %v before it was answered\n", id, obj["status"])
				return 1
			}
			if time.Now().After(deadline) {
				fmt.Fprintf(stdout, "error: timed out waiting for form %s to be answered (status %v)\n", id, obj["status"])
				return 1
			}
			time.Sleep(*interval)
		}
	case "set":
		id, ok := needID()
		if !ok {
			return 2
		}
		def, code := loadDef()
		if code != 0 {
			return code
		}
		payload := map[string]any{"id": id, "title": *title, "body": *body,
			"close_at": *closeAt, "results_visible": *resultsVisible}
		if def != nil {
			payload["definition"] = def
		}
		if len(answers) > 0 {
			ans := map[string]any{}
			for k, v := range answers {
				ans[k] = v
			}
			payload["answers"] = ans
		}
		obj, _, code := call("set", payload)
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
		id, ok := needID()
		if !ok {
			return 2
		}
		inv := *invite
		if inv == "" && len(positional) >= 2 {
			inv = positional[1]
		}
		if inv == "" {
			fmt.Fprintln(stdout, "error: form join needs --invite CODE")
			return 2
		}
		obj, _, code := call("join", map[string]any{"id": id, "invite": inv})
		if code != 0 {
			return code
		}
		return emit(obj, nil)
	default:
		return kit.UsageError(stdout, "form", *asJSON, fmt.Sprintf("unknown verb %q", verb))
	}
}

func looksLikeDefArg(s string) bool {
	if s == "-" || strings.ContainsAny(s, "./\\") {
		return true
	}
	fi, err := os.Stat(s)
	return err == nil && !fi.IsDir()
}

var cliBoolFlags = map[string]bool{
	"json": true, "anonymous": true, "results-visible": true, "complete": true,
	"h": true, "help": true,
}

// splitVerb splits argv into flags before the verb, the verb, and args after
// the verb at the first non-flag token. A lone "-" or "--" counts as the verb
// slot so `-` (stdin) still reaches --def.
func splitVerb(args []string) (pre []string, verb string, post []string) {
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			if i+1 < len(args) {
				return args[:i], args[i+1], args[i+2:]
			}
			return args[:i], "", nil
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			return pre, a, args[i+1:]
		}
		// A flag=value pair or a boolean flag stands alone; any other
		// flag consumes the next token, so a value like "add" after
		// -addr is not mistaken for the verb.
		if strings.Contains(a, "=") || cliBoolFlags[strings.TrimLeft(a, "-")] {
			pre = append(pre, a)
			i++
			continue
		}
		pre = append(pre, a)
		if i+1 < len(args) {
			pre = append(pre, args[i+1])
			i += 2
		} else {
			i++
		}
	}
	return pre, "", nil
}

func parseVerbArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	pos, _, err := parseVerbArgsWithSet(fs, args)
	return pos, err
}

// parseVerbArgsWithSet parses verb arguments in any order (form show ID --json):
// known flags are set on fs wherever they appear, every other token is a
// positional. Unknown flags are an error, never silently ignored.
func parseVerbArgsWithSet(fs *flag.FlagSet, args []string) ([]string, map[string]bool, error) {
	valued := map[string]bool{
		"extract": true, "values": true, "refusals": true,
		"addr": true, "data": true, "prefix": true, "title": true,
		"body": true, "id": true, "def": true, "definition": true,
		"key": true, "timeout": true, "interval": true, "answer": true,
		"answers": true, "participant": true,
		"poll": true, "options": true, "respondents": true, "close-at": true,
		"invite": true,
	}
	visited := map[string]bool{}
	var positional []string
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if a == "-" || !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			i++
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		f := fs.Lookup(name)
		if f == nil {
			return nil, nil, fmt.Errorf("unknown flag --%s", name)
		}
		visited[name] = true
		if !valued[name] {
			if hasVal {
				if err := f.Value.Set(val); err != nil {
					return nil, nil, fmt.Errorf("invalid --%s: %v", name, err)
				}
			} else {
				if err := f.Value.Set("true"); err != nil {
					return nil, nil, fmt.Errorf("invalid --%s: %v", name, err)
				}
			}
			i++
			continue
		}
		if !hasVal {
			i++
			if i >= len(args) {
				return nil, nil, fmt.Errorf("flag --%s needs a value", name)
			}
			val = args[i]
		}
		if err := f.Value.Set(val); err != nil {
			return nil, nil, fmt.Errorf("invalid --%s: %v", name, err)
		}
		i++
	}
	return positional, visited, nil
}

func readDefinitionBytes(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func commonQuery(fields string) string {
	if fields == "" {
		return ""
	}
	return "?fields=" + url.QueryEscape(fields)
}

func serverKeyScope(store, addr, prefix string) string {
	if store != "" && kit.ValidName(store) {
		return store
	}
	return storeID("addr:" + addr + prefix)
}

func clientKeyPath(dataDir, scope, id string) string {
	return filepath.Join(dataDir, "keys", scope, id+".json")
}

func saveClientOwnerKey(dataDir, scope, id, owner string) {
	if dataDir == "" || scope == "" || !kit.ValidName(id) || owner == "" {
		return
	}
	raw, err := json.Marshal(map[string]string{"owner": owner})
	if err != nil {
		return
	}
	_ = kit.AtomicWrite(clientKeyPath(dataDir, scope, id), raw)
}

func readSavedOwner(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var saved struct {
		Owner string `json:"owner"`
	}
	if json.Unmarshal(raw, &saved) == nil {
		return saved.Owner
	}
	return ""
}

func probeServerStore(addr, prefix string) (string, bool) {
	if err := kit.CheckLoopback(addr); err != nil {
		return "", false
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get("http://" + addr + prefix + "/healthz")
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if s := strings.TrimSpace(resp.Header.Get(storeHeader)); s != "" {
		return s, true
	}
	var v struct {
		Store string `json:"store"`
	}
	if json.Unmarshal(buf, &v) == nil && v.Store != "" {
		return v.Store, true
	}
	return "", true
}
