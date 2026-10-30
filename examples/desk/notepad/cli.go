// The notepad agent door: add|open|show|set|list|share|close over the same
// JSON API the page uses. --json prints the raw API object; without it,
// open/new/put/close/list print only id, URL and status — never content.
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

	kit "desk-kit"
)

type cliOptions struct {
	addr       string
	data       string
	prefix     string
	asJSON     bool
	title      string
	body       string
	fileFlag   string
	format     string
	toFlag     string
	outputFlag string
	idFlag     string
	key        string
	breakFlag  string
	verb       string
	args       []string
}

func parseCLIOptions(rawArgs []string) (*cliOptions, error) {
	defaultAddr := "127.0.0.1:18095"
	if envAddr := os.Getenv("NOTEPAD_ADDR"); envAddr != "" {
		defaultAddr = envAddr
	}
	defaultData := kit.DefaultDataDir("notepad")
	if envData := os.Getenv("NOTEPAD_DATA"); envData != "" {
		defaultData = envData
	}
	opts := &cliOptions{
		addr:   defaultAddr,
		data:   defaultData,
		prefix: "",
		key:    os.Getenv("NOTEPAD_KEY"),
	}

	var positional []string
	i := 0
	for i < len(rawArgs) {
		arg := rawArgs[i]
		if arg == "-h" || arg == "--help" {
			return nil, flag.ErrHelp
		}
		if strings.HasPrefix(arg, "-") {
			name := strings.TrimLeft(arg, "-")
			val := ""
			hasVal := false
			if eq := strings.Index(name, "="); eq != -1 {
				val = name[eq+1:]
				name = name[:eq]
				hasVal = true
			}
			setVal := func() (string, bool) {
				if hasVal {
					return val, true
				}
				if i+1 < len(rawArgs) {
					i++
					return rawArgs[i], true
				}
				return "", false
			}
			switch name {
			case "addr":
				if v, ok := setVal(); ok {
					opts.addr = v
				}
			case "data":
				if v, ok := setVal(); ok {
					opts.data = v
				}
			case "prefix":
				if v, ok := setVal(); ok {
					opts.prefix = v
				}
			case "title":
				if v, ok := setVal(); ok {
					opts.title = v
				}
			case "body":
				if v, ok := setVal(); ok {
					opts.body = v
				}
			case "f", "file":
				if v, ok := setVal(); ok {
					opts.fileFlag = v
				}
			case "format":
				if v, ok := setVal(); ok {
					opts.format = v
				}
			case "to":
				if v, ok := setVal(); ok {
					opts.toFlag = v
				}
			case "o", "output":
				if v, ok := setVal(); ok {
					opts.outputFlag = v
				}
			case "id":
				if v, ok := setVal(); ok {
					opts.idFlag = v
				}
			case "key":
				if v, ok := setVal(); ok {
					opts.key = v
				}
			case "break":
				if v, ok := setVal(); ok {
					opts.breakFlag = v
				}
			case "json":
				opts.asJSON = true
			}
			i++
			continue
		}
		positional = append(positional, arg)
		i++
	}

	if len(positional) > 0 {
		opts.verb = positional[0]
		opts.args = positional[1:]
	}
	return opts, nil
}

func runCLI(args []string, stdout io.Writer) int {
	return runCLIWithIO(args, os.Stdin, stdout)
}

func runCLIWithIO(args []string, stdin io.Reader, stdout io.Writer) int {
	var common kit.CommonOptions
	var commonErr error
	args, common, commonErr = kit.ParseCommon(args)
	if commonErr != nil {
		fmt.Fprintln(stdout, "error:", commonErr)
		return 2
	}
	if common.HelpMCP {
		fmt.Fprintln(stdout, kit.CanonicalJSON(kit.MCPHelp("notepad")))
		return 0
	}
	if common.Explain || common.DryRun {
		verb := kit.FindVerb("notepad", args)
		if verb == "" {
			fmt.Fprintln(stdout, "error: unknown verb")
			return 2
		}
		fmt.Fprintln(stdout, kit.CanonicalJSON(kit.Envelope("notepad", map[string]any{"verb": verb, "effects": "desk action", "dry_run": common.DryRun})))
		return 0
	}

	opts, err := parseCLIOptions(args)
	if opts != nil {
		opts.asJSON = opts.asJSON || common.JSON
	}
	if err != nil {
		fmt.Fprintln(stdout, "Usage: notepad [serve|add|open|show|set|list|share|close|join|export] [--json] [-f FILE|-] [--to FORMAT] [-o FILE]")
		return 2
	}
	if opts.verb == "" {
		fmt.Fprintln(stdout, "Usage: notepad [serve|add|open|show|set|list|share|close|join|export] [--json] [-f FILE|-] [--to FORMAT] [-o FILE]")
		return 2
	}

	if opts.verb == "serve" {
		breakMode = opts.breakFlag
		serve(opts.addr, opts.data, opts.prefix)
		return 0
	}

	loadContent := func(fPath, fFmt string) (string, string, error) {
		var raw []byte
		var err error
		var autoTitle string
		if fPath == "-" {
			raw, err = io.ReadAll(stdin)
			if err != nil {
				return "", "", fmt.Errorf("reading stdin: %w", err)
			}
		} else {
			raw, err = os.ReadFile(fPath)
			if err != nil {
				return "", "", fmt.Errorf("reading file %s: %w", fPath, err)
			}
			base := filepath.Base(fPath)
			autoTitle = strings.TrimSuffix(base, filepath.Ext(base))
		}
		conv, _, err := ConvertContent(raw, fFmt+fPath)
		if err != nil {
			return "", "", fmt.Errorf("converting content: %w", err)
		}
		return conv, autoTitle, nil
	}

	// Helper to lookup owner key from local store when --key is omitted
	resolveKey := func(id string) {
		if opts.key != "" || id == "" {
			return
		}
		itemF := filepath.Join(opts.data, "items", id+".json")
		if raw, err := os.ReadFile(itemF); err == nil {
			var saved struct {
				Owner string `json:"owner"`
			}
			if json.Unmarshal(raw, &saved) == nil && saved.Owner != "" {
				opts.key = saved.Owner
			}
		}
	}

	base := "http://" + opts.addr + opts.prefix
	call := func(action string, payload map[string]any) (map[string]any, []map[string]any, int) {
		payload["action"] = action
		raw, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", base+"/api/action"+commonQuery(common.Fields), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if opts.key != "" {
			req.Header.Set("X-Facilitator-Key", opts.key)
		}
		client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}}
		r, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(stdout, "error: notepad server at %s is not reachable: %v\n", base, err)
			return nil, nil, -1
		}
		defer r.Body.Close()
		buf, _ := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if r.StatusCode != 200 {
			if opts.asJSON {
				fmt.Fprintln(stdout, string(buf))
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

	emit := func(obj map[string]any, arr []map[string]any) int {
		if opts.asJSON {
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
		switch opts.verb {
		case "show":
			fmt.Fprintf(stdout, "%v\n%v\n", obj["title"], obj["body"])
		case "export":
			fmt.Fprintf(stdout, "%v\n", obj["path"])
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
		if len(opts.args) == 0 {
			if opts.idFlag != "" && kit.ValidName(opts.idFlag) {
				return opts.idFlag, true
			}
			fmt.Fprintln(stdout, "error: give the note id")
			return "", false
		}
		if !kit.ValidName(opts.args[0]) {
			fmt.Fprintln(stdout, "error: use 1-64 letters, digits, - or _ for an id")
			return "", false
		}
		return opts.args[0], true
	}

	switch opts.verb {
	case "add":
		content := opts.body
		noteTitle := opts.title
		idVal := opts.idFlag
		if idVal == "" && len(opts.args) > 0 && kit.ValidName(opts.args[0]) {
			idVal = opts.args[0]
		}
		if opts.fileFlag != "" {
			c, autoT, err := loadContent(opts.fileFlag, opts.format)
			if err != nil {
				fmt.Fprintf(stdout, "error: %v\n", err)
				return 1
			}
			content = c
			if noteTitle == "" && autoT != "" {
				noteTitle = autoT
			}
		}
		payload := map[string]any{"id": idVal, "title": noteTitle, "body": content}
		obj, _, code := call("add", payload)
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
		resolveKey(id)
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
		resolveKey(id)
		content := opts.body
		noteTitle := opts.title
		if opts.fileFlag != "" {
			c, autoT, err := loadContent(opts.fileFlag, opts.format)
			if err != nil {
				fmt.Fprintf(stdout, "error: %v\n", err)
				return 1
			}
			content = c
			if noteTitle == "" && autoT != "" {
				noteTitle = autoT
			}
		}
		payload := map[string]any{"id": id, "title": noteTitle, "body": content}
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
		resolveKey(id)
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
		resolveKey(id)
		obj, _, code := call("close", map[string]any{"id": id})
		if code != 0 {
			return code
		}
		return emit(obj, nil)

	case "export":
		id, ok := needID()
		if !ok {
			return 2
		}
		resolveKey(id)
		toFormat := opts.toFlag
		if toFormat == "" {
			toFormat = opts.format
		}
		if toFormat == "" {
			fmt.Fprintln(stdout, "error: specify export format with --to pdf|png|html|txt")
			return 2
		}
		toFormat = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(toFormat), "."))
		switch toFormat {
		case "pdf", "png", "html", "htm", "txt", "text", "md", "markdown":
			// valid
		default:
			fmt.Fprintf(stdout, "error: unsupported export format %q (supported: pdf, png, html, txt)\n", toFormat)
			return 2
		}
		outPath := opts.outputFlag
		if outPath != "" {
			abs, err := filepath.Abs(outPath)
			if err == nil {
				outPath = abs
			}
		}
		payload := map[string]any{
			"id":     id,
			"to":     toFormat,
			"output": outPath,
		}
		obj, _, code := call("export", payload)
		if code != 0 {
			return code
		}
		return emit(obj, nil)

	case "join":
		if len(opts.args) < 2 {
			fmt.Fprintln(stdout, "error: notepad join ID INVITE")
			return 2
		}
		u := base + "/?id=" + url.QueryEscape(opts.args[0]) + "#invite=" + opts.args[1]
		fmt.Fprintln(stdout, u)
		return 0

	default:
		return kit.UsageError(stdout, "notepad", opts.asJSON, fmt.Sprintf("unknown verb %q", opts.verb))
	}
}

func commonQuery(fields string) string {
	if fields == "" {
		return ""
	}
	return "?fields=" + url.QueryEscape(fields)
}
