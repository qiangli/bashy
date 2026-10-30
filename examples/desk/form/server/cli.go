// The form agent door: new|open|get|put|list|share|close|wait over the
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
	"strings"
	"time"

	kit "desk-kit"
)

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

func runCLI(args []string, stdout io.Writer) int {
	// The verb comes first (form get ID --json); connection flags may
	// precede it (form -addr 127.0.0.1:18107 get ID). Split the argv at
	// the first non-flag token so flags work on either side.
	pre, verb, post := splitVerb(args)
	if verb == "" {
		fmt.Fprintln(stdout, "Usage: form [serve|new|open|get|put|list|share|close|wait] [--json]")
		return 2
	}
	gfs := flag.NewFlagSet("form", flag.ContinueOnError)
	gaddr := gfs.String("addr", "127.0.0.1:18107", "tool server address")
	gdata := gfs.String("data", kit.DefaultDataDir("form"), "data directory")
	gprefix := gfs.String("prefix", "", "URL prefix when served behind a sub-path")
	if err := gfs.Parse(pre); err != nil {
		return 2
	}
	fs := flag.NewFlagSet("form "+verb, flag.ContinueOnError)
	addr := fs.String("addr", *gaddr, "tool server address")
	data := fs.String("data", *gdata, "data directory")
	prefix := fs.String("prefix", *gprefix, "URL prefix when served behind a sub-path")
	asJSON := fs.Bool("json", false, "print the raw JSON API object")
	title := fs.String("title", "", "form title")
	body := fs.String("body", "", "form body (agent note, returned by get)")
	idFlag := fs.String("id", "", "form id (new)")
	defPath := fs.String("def", "", "form definition file (JSON or YAML; - for stdin)")
	defInline := fs.String("definition", "", "form definition inline (JSON or YAML)")
	key := fs.String("key", "", "owner key (FORM_KEY also works)")
	timeout := fs.Duration("timeout", 5*time.Minute, "wait: give up after this long")
	interval := fs.Duration("interval", 2*time.Second, "wait: poll interval")
	answers := answerFlags{}
	fs.Var(answers, "answer", "prefill answer as key=value (repeatable, with put)")
	positional, err := parseVerbArgs(fs, post)
	if err != nil {
		fmt.Fprintf(stdout, "error: %v\n", err)
		return 2
	}
	if *key == "" {
		*key = os.Getenv("FORM_KEY")
	}
	if verb == "serve" {
		serve(*addr, *data, *prefix)
		return 0
	}
	// A definition for new/put comes from --def, --definition, or neither.
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
		t, def, err := ParseDefinition([]byte(src), name)
		if err != nil {
			fmt.Fprintf(stdout, "error: %v\n", err)
			return nil, 1
		}
		if *title == "" {
			*title = t
		}
		raw, _ := json.Marshal(def)
		var v any
		_ = json.Unmarshal(raw, &v)
		return v, 0
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
			fmt.Fprintf(stdout, "error: form server at %s is not reachable: %v\n", base, err)
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
		case "get", "wait":
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
	switch verb {
	case "new":
		def, code := loadDef()
		if code != 0 {
			return code
		}
		payload := map[string]any{"id": *idFlag, "title": *title, "body": *body}
		if def != nil {
			payload["definition"] = def
		}
		obj, _, code := call("new", payload)
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
	case "get":
		id, ok := needID()
		if !ok {
			return 2
		}
		obj, _, code := call("get", map[string]any{"id": id})
		if code != 0 {
			return code
		}
		return emit(obj, nil)
	case "wait":
		id, ok := needID()
		if !ok {
			return 2
		}
		deadline := time.Now().Add(*timeout)
		for {
			obj, _, code := call("get", map[string]any{"id": id})
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
	case "put":
		id, ok := needID()
		if !ok {
			return 2
		}
		def, code := loadDef()
		if code != 0 {
			return code
		}
		payload := map[string]any{"id": id, "title": *title, "body": *body}
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
		obj, _, code := call("put", payload)
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
		if len(positional) < 2 {
			fmt.Fprintln(stdout, "error: form join ID INVITE")
			return 2
		}
		u := base + "/?id=" + url.QueryEscape(positional[0]) + "#invite=" + positional[1]
		fmt.Fprintln(stdout, u)
		return 0
	default:
		fmt.Fprintf(stdout, "error: unknown verb %q\n", verb)
		return 2
	}
}

// splitVerb splits argv into connection flags, the verb, and verb args at
// the first non-flag token. A lone "-" or "--" counts as the verb slot so
// `-` (stdin) still reaches --def.
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
		// flag consumes the next token, so a value like "new" after
		// -addr is not mistaken for the verb.
		if strings.Contains(a, "=") || strings.TrimLeft(a, "-") == "json" {
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

// parseVerbArgs parses verb arguments in any order (form get ID --json):
// known flags are set on fs wherever they appear, every other token is a
// positional. Unknown flags are an error, never silently ignored.
func parseVerbArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	valued := map[string]bool{
		"addr": true, "data": true, "prefix": true, "title": true,
		"body": true, "id": true, "def": true, "definition": true,
		"key": true, "timeout": true, "interval": true, "answer": true,
	}
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
			return nil, fmt.Errorf("unknown flag --%s", name)
		}
		if !valued[name] {
			if hasVal {
				if err := f.Value.Set(val); err != nil {
					return nil, fmt.Errorf("invalid --%s: %v", name, err)
				}
			} else {
				if err := f.Value.Set("true"); err != nil {
					return nil, fmt.Errorf("invalid --%s: %v", name, err)
				}
			}
			i++
			continue
		}
		if !hasVal {
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("flag --%s needs a value", name)
			}
			val = args[i]
		}
		if err := f.Value.Set(val); err != nil {
			return nil, fmt.Errorf("invalid --%s: %v", name, err)
		}
		i++
	}
	return positional, nil
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
