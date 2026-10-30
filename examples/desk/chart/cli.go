package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	kit "desk-kit"
)

// Permit conventional verb ID --flags as well as flags before the verb.
func interspersed(args []string) []string {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		s := args[i]
		if s == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(s, "-") && s != "-" {
			flags = append(flags, s)
			if !strings.Contains(s, "=") && s != "--json" && s != "-json" && s != "--help" && s != "-h" {
				if i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				}
			}
		} else {
			positionals = append(positionals, s)
		}
	}
	return append(flags, positionals...)
}
func runCLI(args []string, stdout io.Writer) int { return cli(args, os.Stdin, stdout) }
func cli(args []string, stdin io.Reader, stdout io.Writer) int {
	fs := flag.NewFlagSet("chart", flag.ContinueOnError)
	fs.SetOutput(stdout)
	addr := fs.String("addr", "127.0.0.1:18098", "loopback server address")
	data := fs.String("data", kit.DefaultDataDir("chart"), "server storage directory")
	prefix := fs.String("prefix", "", "URL path prefix")
	asJSON := fs.Bool("json", false, "JSON output")
	title := fs.String("title", "", "chart title")
	body := fs.String("body", "", "optional description")
	idFlag := fs.String("id", "", "new item id")
	key := fs.String("key", os.Getenv("CHART_KEY"), "owner key (or CHART_KEY)")
	input := fs.String("input", "", "input file, - for stdin, or loopback URL")
	format := fs.String("format", "auto", "auto, json, csv, ndjson or otlp")
	widget := fs.String("widget", "auto", "auto, counter, sum, gauge, updowncounter, histogram, exponentialhistogram, line, bar, table, stat")
	tmpl := fs.String("template", "", "shaped HTML template file")
	refresh := fs.String("refresh", "", "re-read file or loopback URL, e.g. 5s")
	if e := fs.Parse(interspersed(args)); e != nil {
		if e == flag.ErrHelp {
			return 0
		}
		return 2
	}
	fail := func(e error) int {
		if *asJSON {
			_ = json.NewEncoder(stdout).Encode(map[string]string{"error": e.Error()})
		} else {
			fmt.Fprintln(stdout, "error:", e)
		}
		return 1
	}
	if e := kit.CheckLoopback(*addr); e != nil {
		return fail(e)
	}
	if *prefix != "" && (!strings.HasPrefix(*prefix, "/") || strings.HasSuffix(*prefix, "/") || strings.ContainsAny(*prefix, "?#")) {
		return fail(fmt.Errorf("prefix must be a path like /chart"))
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(stdout, "chart serve|new|open|get|put|list|share|close|revoke [ID] --json; new/put --input FILE|- --widget TYPE")
		return 2
	}
	verb := rest[0]
	if verb == "serve" {
		serve(*addr, *data, *prefix)
		return 0
	}
	payload := map[string]any{"action": verb}
	switch verb {
	case "new":
		payload["id"] = *idFlag
	case "open", "get", "put", "share", "close", "revoke":
		if len(rest) != 2 {
			return fail(fmt.Errorf("%s requires one id", verb))
		}
		payload["id"] = rest[1]
	case "list":
	default:
		return fail(fmt.Errorf("unknown verb %q", verb))
	}
	if verb == "new" || verb == "put" {
		payload["title"] = *title
		payload["body"] = *body
		if *input == "" && verb == "new" {
			*input = "-"
		}
		if *input != "" {
			c := chartConfig{Title: *title, Format: *format, Widget: *widget, Refresh: *refresh}
			if *input == "-" {
				if *refresh != "" {
					return fail(fmt.Errorf("stdin cannot refresh"))
				}
				raw, e := readBounded(stdin)
				if e != nil {
					return fail(e)
				}
				c.Input = string(raw)
			} else {
				source := *input
				if !strings.Contains(source, "://") {
					var e error
					source, e = filepath.Abs(source)
					if e != nil {
						return fail(e)
					}
				}
				raw, e := readSource(source)
				if e != nil {
					return fail(e)
				}
				c.Input = string(raw)
				if *refresh != "" {
					c.Source = source
				}
			}
			if *tmpl != "" {
				f, e := os.Open(*tmpl)
				if e != nil {
					return fail(e)
				}
				raw, e := readBounded(f)
				f.Close()
				if e != nil {
					return fail(e)
				}
				c.Template = string(raw)
			}
			payload["chart"] = c
		} else if *refresh != "" || *tmpl != "" || *widget != "auto" {
			return fail(fmt.Errorf("chart options require --input"))
		}
	}
	raw, e := json.Marshal(payload)
	if e != nil {
		return fail(e)
	}
	req, e := http.NewRequest("POST", "http://"+*addr+*prefix+"/api/action", bytes.NewReader(raw))
	if e != nil {
		return fail(e)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Facilitator-Key", *key)
	client := http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("redirect refused") }}
	res, e := client.Do(req)
	if e != nil {
		return fail(e)
	}
	defer res.Body.Close()
	var v any
	if e = json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(&v); e != nil {
		return fail(e)
	}
	if res.StatusCode != 200 {
		m, _ := v.(map[string]any)
		return fail(fmt.Errorf("%v", m["error"]))
	}
	if *asJSON || verb == "get" || verb == "list" {
		return encode(stdout, v)
	}
	m := v.(map[string]any)
	fmt.Fprintf(stdout, "%v %v %v\n", m["id"], m["url"], m["status"])
	for _, k := range []string{"ownerKey", "inviteURL"} {
		if s, ok := m[k]; ok {
			fmt.Fprintf(stdout, "%s: %v\n", k, s)
		}
	}
	return 0
}
func encode(w io.Writer, v any) int {
	if json.NewEncoder(w).Encode(v) != nil {
		return 1
	}
	return 0
}
