package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	kit "desk-kit"
)

// The desk agent door: `whiteboard-server cli [flags] add|open|show|set|list|share|close`.
// It speaks the same /api/action route as the page. open/new/put/share/close
// print id, URL and status only; get prints the board content as JSON.
func runCLI(args []string, stdout io.Writer) int {
	var common kit.CommonOptions
	var commonErr error
	args, common, commonErr = kit.ParseCommon(args)
	if commonErr != nil {
		fmt.Fprintln(stdout, "error:", commonErr)
		return 2
	}
	if common.HelpMCP {
		help := kit.MCPHelp("whiteboard")
		help["verbs"] = append(kit.ToolVerbs("whiteboard"), "draw", "clear", "lock", "revoke", "download")
		fmt.Fprintln(stdout, kit.CanonicalJSON(help))
		return 0
	}
	if common.Explain || common.DryRun {
		verb := kit.FindVerb("whiteboard", args)
		if verb == "" {
			for _, a := range args {
				if strings.Contains(" draw clear lock revoke download ", " "+a+" ") {
					verb = a
					break
				}
			}
		}
		if verb == "" {
			fmt.Fprintln(stdout, "error: unknown verb")
			return 2
		}
		fmt.Fprintln(stdout, kit.CanonicalJSON(kit.Envelope("whiteboard", map[string]any{"verb": verb, "effects": "desk action", "dry_run": common.DryRun})))
		return 0
	}

	fs := flag.NewFlagSet("whiteboard", flag.ContinueOnError)
	fs.SetOutput(stdout)
	addr := fs.String("addr", "127.0.0.1:18093", "server address")
	prefix := fs.String("prefix", "", "URL prefix when served behind a sub-path")
	asJSON := fs.Bool("json", common.JSON, "print the raw JSON API object")
	title := fs.String("title", "", "accepted for desk compatibility; boards have no title")
	body := fs.String("body", "", "note text (new, put)")
	idFlag := fs.String("id", "", "board id (new)")
	key := fs.String("key", "", "facilitator key (WHITEBOARD_KEY also works)")
	participant := fs.String("participant", "", "stable participant token for member actions")
	points := fs.String("points", "", "draw points as JSON [{x,y},...]")
	color := fs.String("color", "#222222", "stroke color")
	width := fs.Float64("width", 3, "stroke width")
	locked := fs.Bool("locked", true, "lock state; use --locked=false to unlock")
	target := fs.String("target", "", "member or invite code to revoke")
	format := fs.String("format", "json", "download format: json or png")
	output := fs.String("output", "", "save download to file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stdout, "Usage: whiteboard [serve|add|open|show|set|list|share|close|draw|clear|lock|revoke|download] [--json]")
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
	if !deskActions[verb] && !strings.Contains(" draw clear lock revoke download ", " "+verb+" ") {
		return kit.UsageError(stdout, "whiteboard", *asJSON, fmt.Sprintf("unknown verb %q", verb))
	}
	if *key == "" {
		*key = os.Getenv("WHITEBOARD_KEY")
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
	if verb == "draw" {
		payload["action"] = "stroke"
	}
	switch verb {
	case "list":
	case "add":
		payload["id"], payload["title"], payload["body"] = *idFlag, *title, *body
	default:
		if len(positional) == 0 {
			fmt.Fprintln(stdout, "error: give the board id")
			return 2
		}
		if !kit.ValidName(positional[0]) {
			fmt.Fprintln(stdout, "error: use 1-64 letters, digits, - or _ for an id")
			return 2
		}
		payload["id"] = positional[0]
		if verb == "draw" || verb == "clear" || verb == "lock" || verb == "revoke" {
			payload["board"] = positional[0]
		}
		if verb == "draw" {
			var pts []Pt
			if err := json.Unmarshal([]byte(*points), &pts); err != nil || len(pts) == 0 {
				return kit.UsageError(stdout, "whiteboard", *asJSON, "draw requires --points JSON array")
			}
			payload["points"], payload["color"], payload["width"] = pts, *color, *width
		}
		if verb == "lock" {
			payload["locked"] = *locked
		}
		if verb == "revoke" {
			if *target == "" {
				return kit.UsageError(stdout, "whiteboard", *asJSON, "revoke requires --target MEMBER_OR_INVITE")
			}
			payload["id"] = *target
		}
		if verb == "set" {
			payload["body"] = *body
		}
	}
	if verb == "download" {
		if *format != "json" && *format != "png" {
			return kit.UsageError(stdout, "whiteboard", *asJSON, "format must be json or png")
		}
		u := base + "/api/export?board=" + url.QueryEscape(positional[0]) + "&format=" + *format
		req, _ := http.NewRequest("GET", u, nil)
		if *participant != "" {
			req.AddCookie(&http.Cookie{Name: "whiteboard-participant", Value: *participant})
		}
		if *key != "" {
			req.Header.Set("X-Facilitator-Key", *key)
		}
		r, err := (&http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}}).Do(req)
		if err != nil {
			fmt.Fprintln(stdout, "error:", err)
			return 1
		}
		defer r.Body.Close()
		buf, _ := io.ReadAll(io.LimitReader(r.Body, 16<<20))
		if r.StatusCode != 200 {
			if *asJSON {
				fmt.Fprintln(stdout, string(buf))
			} else {
				fmt.Fprintln(stdout, "error:", kit.ErrorMessage(buf))
			}
			return 1
		}
		if *output != "" {
			if err := os.WriteFile(*output, buf, 0600); err != nil {
				fmt.Fprintln(stdout, "error:", err)
				return 1
			}
			fmt.Fprintln(stdout, kit.CanonicalJSON(kit.Envelope("whiteboard", map[string]any{"id": positional[0], "format": *format, "output": *output})))
			return 0
		}
		if *format == "png" {
			fmt.Fprintln(stdout, kit.CanonicalJSON(kit.Envelope("whiteboard", map[string]any{"id": positional[0], "format": "png", "data_base64": base64.StdEncoding.EncodeToString(buf)})))
		} else {
			fmt.Fprintln(stdout, string(buf))
		}
		return 0
	}
	raw, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", base+"/api/action"+commonQuery(common.Fields), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if *participant != "" {
		req.AddCookie(&http.Cookie{Name: "whiteboard-participant", Value: *participant})
	}
	if *key != "" {
		req.Header.Set("X-Facilitator-Key", *key)
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil}}
	r, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(stdout, "error: whiteboard server at %s is not reachable: %v\n", base, err)
		return 1
	}
	defer r.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if r.StatusCode != 200 {
		if *asJSON {
			fmt.Fprintln(stdout, string(buf))
		} else {
			fmt.Fprintf(stdout, "error: %s\n", kit.ErrorMessage(buf))
		}
		if r.StatusCode == http.StatusBadRequest {
			return 2
		}
		return 1
	}
	if common.Fields != "" && *asJSON {
		fmt.Fprintln(stdout, string(buf))
		return 0
	}
	if verb == "list" {
		var envelope struct {
			Items []map[string]any `json:"items"`
		}
		if json.Unmarshal(buf, &envelope) != nil || envelope.Items == nil {
			fmt.Fprintf(stdout, "error: invalid server response: %s\n", buf)
			return 1
		}
		if *asJSON {
			fmt.Fprintln(stdout, string(buf))
			return 0
		}
		for _, e := range envelope.Items {
			fmt.Fprintf(stdout, "%v %v %v\n", e["id"], e["status"], e["url"])
		}
		return 0
	}
	var obj map[string]any
	if json.Unmarshal(buf, &obj) != nil {
		fmt.Fprintln(stdout, "error: invalid server response")
		return 1
	}
	if *asJSON || verb == "show" {
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

func commonQuery(fields string) string {
	if fields == "" {
		return ""
	}
	return "?fields=" + url.QueryEscape(fields)
}
