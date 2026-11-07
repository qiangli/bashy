package main

import (
	"bytes"
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

type multiStringFlag []string

func (m *multiStringFlag) String() string { return strings.Join(*m, "\n") }
func (m *multiStringFlag) Set(v string) error {
	for _, line := range strings.Split(v, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			*m = append(*m, s)
		}
	}
	return nil
}

// The desk agent door: `brainstorm-server cli [flags] add|open|show|set|list|share|close`.
// It speaks the same /api/action route as the page. open/new/put/share/close
// print id, URL and status only; get prints the session content as JSON.
func runCLI(args []string, stdout io.Writer) int {
	var common kit.CommonOptions
	var commonErr error
	args, common, commonErr = kit.ParseCommon(args)
	if commonErr != nil {
		fmt.Fprintln(stdout, "error:", commonErr)
		return 2
	}
	if common.HelpMCP {
		fmt.Fprintln(stdout, kit.CanonicalJSON(kit.MCPHelp("brainstorm")))
		return 0
	}
	if common.Explain || common.DryRun {
		verb := kit.FindVerb("brainstorm", args)
		if verb == "" {
			fmt.Fprintln(stdout, "error: unknown verb")
			return 2
		}
		fmt.Fprintln(stdout, kit.CanonicalJSON(kit.Envelope("brainstorm", map[string]any{"verb": verb, "effects": "desk action", "dry_run": common.DryRun})))
		return 0
	}

	fs := flag.NewFlagSet("brainstorm", flag.ContinueOnError)
	fs.SetOutput(stdout)
	addr := fs.String("addr", "127.0.0.1:18091", "server address")
	prefix := fs.String("prefix", "", "URL prefix when served behind a sub-path")
	asJSON := fs.Bool("json", common.JSON, "print the raw JSON API object")
	title := fs.String("title", "", "accepted for desk compatibility; session title (new)")
	body := fs.String("body", "", "idea text (new, put)")
	idFlag := fs.String("id", "", "session id (new)")
	key := fs.String("key", "", "facilitator key (BRAINSTORM_KEY also works)")
	column := fs.Int("column", 0, "column index for preset card (0-based)")
	var presets multiStringFlag
	fs.Var(&presets, "preset", "preset seed card text (repeatable or newline-separated)")
	presetCard := fs.Bool("preset-card", false, "mark --body as a preset seed card visible during write")
	priorVotes := fs.Int("prior-votes", 0, "prior vote tally for preset card")
	fromSession := fs.String("from-session", "", "seed cards from an existing saved session")
	fromRound := fs.Int("from-round", 0, "round number when seeding from --from-session (0 = current)")
	fromKey := fs.String("from-key", "", "facilitator key of --from-session if private")
	roundFlag := fs.Int("round", 0, "inspect a specific round in show (0 = current)")
	phaseFlag := fs.String("phase", "", "advance phase (discuss|vote|closed)")
	nextRound := fs.Bool("next-round", false, "snapshot current round and advance to the next round")
	startPhase := fs.String("start-phase", "", "phase to start new session or next round in (write|discuss|vote)")
	minVotes := fs.Int("min-votes", 0, "minimum votes required to advance/import cards")
	topN := fs.Int("top-n", 0, "advance/import top N cards by vote count")
	selectIDs := fs.String("select", "", "comma-separated card IDs to advance/import manually")
	filterColumn := fs.Int("filter-column", -1, "only advance/import cards from this column index (>= 0)")
	filterAuthor := fs.String("filter-author", "", "only advance/import cards matching this author")
	filterQuery := fs.String("filter-query", "", "only advance/import cards matching this text query")
	keepVotes := fs.Bool("keep-votes", false, "retain current round votes when advancing/importing")
	reopenRound := fs.Int("reopen-round", 0, "reopen/revert to an earlier completed round number")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stdout, "Usage: brainstorm [serve|add|open|show|set|list|share|close] [--json]")
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
		return kit.UsageError(stdout, "brainstorm", *asJSON, fmt.Sprintf("unknown verb %q", verb))
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

	applySeedAndFilter := func(p map[string]any) {
		if len(presets) > 0 {
			items := make([]PresetCardInput, 0, len(presets))
			for _, text := range presets {
				items = append(items, PresetCardInput{Column: *column, Text: text, PriorVotes: *priorVotes})
			}
			p["presets"] = items
		}
		if *presetCard {
			p["preset"] = true
			p["column"] = *column
			if *priorVotes > 0 {
				p["priorVotes"] = *priorVotes
			}
		}
		if *fromSession != "" {
			p["fromSession"] = *fromSession
			if *fromRound > 0 {
				p["fromRound"] = *fromRound
			}
			if *fromKey != "" {
				p["fromKey"] = *fromKey
			}
		}
		if *startPhase != "" {
			p["startPhase"] = *startPhase
		}
		if *minVotes > 0 {
			p["minVotes"] = *minVotes
		}
		if *topN > 0 {
			p["topN"] = *topN
		}
		if *selectIDs != "" {
			var ids []string
			for _, id := range strings.Split(*selectIDs, ",") {
				if s := strings.TrimSpace(id); s != "" {
					ids = append(ids, s)
				}
			}
			if len(ids) > 0 {
				p["selectedIds"] = ids
			}
		}
		if *filterColumn >= 0 {
			p["filterColumn"] = *filterColumn
		}
		if *filterAuthor != "" {
			p["filterAuthor"] = *filterAuthor
		}
		if *filterQuery != "" {
			p["filterQuery"] = *filterQuery
		}
		if *keepVotes {
			p["keepVotes"] = true
		}
	}

	payload := map[string]any{"action": verb}
	switch verb {
	case "list":
	case "add":
		payload["id"], payload["title"], payload["body"] = *idFlag, *title, *body
		applySeedAndFilter(payload)
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
		if verb == "show" && *roundFlag > 0 {
			payload["round"] = *roundFlag
		}
		if verb == "set" {
			payload["body"] = *body
			if *phaseFlag != "" {
				payload["phase"] = *phaseFlag
			}
			if *nextRound {
				payload["nextRound"] = true
			}
			if *reopenRound > 0 {
				payload["reopenRound"] = *reopenRound
			}
			applySeedAndFilter(payload)
		}
	}
	raw, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", base+"/api/action"+commonQuery(common.Fields), bytes.NewReader(raw))
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
