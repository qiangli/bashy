// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

// `bashy activity` — the control and inspection surface for the shared
// activity-event contract (internal/agentos/activity, docs/activity-events.md).
//
// It is deliberately NOT a read surface for recipients. A recipient reads its
// activity in `bashy inbox`, alongside everything else addressed to it, because
// the contract publishes onto the same durable bus every other source uses and
// adding a second place to look would recreate the fragmentation the unified
// inbox exists to end. What lives here is what the inbox cannot answer: what am
// I subscribed to, why did that event reach me, and what is owed.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/qiangli/bashy/internal/agentos/activity"
)

func dispatchActivity(args []string) int {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
		args = args[1:]
	}
	switch sub {
	case "status":
		return activityStatus(os.Stdout, args)
	case "subscribe":
		return activitySubscribe(args)
	case "unsubscribe":
		return activityUnsubscribe(args)
	case "interests":
		return activityInterests(os.Stdout, args)
	case "tail":
		return activityTail(os.Stdout, args)
	case "show":
		return activityShow(os.Stdout, args)
	case "routes":
		return activityRoutes(os.Stdout, args)
	case "recover":
		return activityRecover(os.Stdout, args)
	case "sources":
		for _, s := range activity.Sources() {
			fmt.Println(s)
		}
		return 0
	case "-h", "--help", "help":
		activityUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "bashy activity: unknown subcommand %q (try: status subscribe unsubscribe interests tail show routes recover sources)\n", sub)
		return 2
	}
}

func activityUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: bashy activity {status|subscribe|unsubscribe|interests|tail|show|routes|recover|sources}")
	fmt.Fprintln(w, "  status              delivery health: journal size, owed deliveries, wake outcomes")
	fmt.Fprintln(w, "  subscribe <who>     declare an interest (--source --noun --action --repo --sprint")
	fmt.Fprintln(w, "                      --topic --object --audit --no-wake --mute --max-wake-per-min)")
	fmt.Fprintln(w, "  unsubscribe <who>   drop a declared interest")
	fmt.Fprintln(w, "  interests           list declared interests")
	fmt.Fprintln(w, "  tail [N]            recent events, compact one line each (--source, --json)")
	fmt.Fprintln(w, "  show <id>           the full envelope behind a rendered `id=` (--json)")
	fmt.Fprintln(w, "  routes <id>         who an event reached and WHY it reached each of them")
	fmt.Fprintln(w, "  recover             re-drive any delivery the journal shows as owed")
	fmt.Fprintln(w, "  sources             the registered subsystem sources")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Recipients read their activity in `bashy inbox`; this verb is the control surface.")
	fmt.Fprintln(w, "`tail` is a compatibility and recovery fallback, not the normal read path.")
}

// activityFlags is a tiny hand-rolled parser. The verb takes repeated
// list-valued flags and this keeps it dependency-free at the dispatch layer,
// matching the other bashy-owned verbs in this package.
type activityFlags struct {
	lists map[string][]string
	bools map[string]bool
	ints  map[string]int
	rest  []string
}

func parseActivityFlags(args []string, listNames, boolNames, intNames []string) (activityFlags, error) {
	f := activityFlags{lists: map[string][]string{}, bools: map[string]bool{}, ints: map[string]int{}}
	isList := map[string]bool{}
	for _, n := range listNames {
		isList[n] = true
	}
	isBool := map[string]bool{}
	for _, n := range boolNames {
		isBool[n] = true
	}
	isInt := map[string]bool{}
	for _, n := range intNames {
		isInt[n] = true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			f.rest = append(f.rest, a)
			continue
		}
		name, value, inline := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		switch {
		case isBool[name]:
			f.bools[name] = true
		case isList[name] || isInt[name]:
			if !inline {
				if i+1 >= len(args) {
					return f, fmt.Errorf("--%s needs a value", name)
				}
				i++
				value = args[i]
			}
			if isInt[name] {
				n, err := strconv.Atoi(value)
				if err != nil {
					return f, fmt.Errorf("--%s: %v", name, err)
				}
				f.ints[name] = n
			} else {
				// Repeatable and comma-separated both work; an agent composing a
				// command line should not have to know which this verb prefers.
				for _, part := range strings.Split(value, ",") {
					if part = strings.TrimSpace(part); part != "" {
						f.lists[name] = append(f.lists[name], part)
					}
				}
			}
		default:
			return f, fmt.Errorf("unknown flag --%s", name)
		}
	}
	return f, nil
}

func activitySubscribe(args []string) int {
	f, err := parseActivityFlags(args,
		[]string{"source", "noun", "action", "repo", "sprint", "topic", "object"},
		[]string{"audit", "no-wake", "mute", "json"},
		[]string{"max-wake-per-min"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy activity subscribe: %v\n", err)
		return 2
	}
	if len(f.rest) != 1 {
		fmt.Fprintln(os.Stderr, "bashy activity subscribe: name exactly one subscriber")
		return 2
	}
	in := activity.Interest{
		Subscriber:    f.rest[0],
		Sources:       f.lists["source"],
		Nouns:         f.lists["noun"],
		Actions:       f.lists["action"],
		Repos:         f.lists["repo"],
		Sprints:       f.lists["sprint"],
		Topics:        f.lists["topic"],
		Objects:       f.lists["object"],
		Audit:         f.bools["audit"],
		Mute:          f.bools["mute"],
		MaxWakePerMin: f.ints["max-wake-per-min"],
		// Wake defaults ON here, unlike bus.Subscription's interrupt rights,
		// because this flag governs whether a MATCH may wake — not who may
		// interrupt. The governance boundary stays where pkg/bus put it: the
		// bus still refuses an interrupt from a principal the subscriber has
		// not named, so a wake this verb permits is not a wake the bus grants.
		Wake: !f.bools["no-wake"],
	}
	if err := activity.Subscribe(in); err != nil {
		fmt.Fprintf(os.Stderr, "bashy activity subscribe: %v\n", err)
		return 1
	}
	if f.bools["json"] {
		return emitJSON(os.Stdout, in)
	}
	fmt.Printf("subscribed: %s\n", describeInterest(in))
	return 0
}

func activityUnsubscribe(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "bashy activity unsubscribe: name exactly one subscriber")
		return 2
	}
	if err := activity.Unsubscribe(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "bashy activity unsubscribe: %v\n", err)
		return 1
	}
	fmt.Printf("unsubscribed: %s\n", args[0])
	return 0
}

func activityInterests(w io.Writer, args []string) int {
	f, _ := parseActivityFlags(args, nil, []string{"json"}, nil)
	in, err := activity.LoadInterests()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy activity interests: %v\n", err)
		return 1
	}
	if f.bools["json"] {
		return emitJSON(w, in)
	}
	if len(in) == 0 {
		fmt.Fprintln(w, "no declared interests")
		return 0
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SUBSCRIBER\tWAKE\tINTEREST")
	for _, i := range in {
		wake := "yes"
		switch {
		case i.Mute:
			wake = "muted"
		case !i.Wake:
			wake = "no"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", i.Subscriber, wake, describeInterest(i))
	}
	return flushTab(tw)
}

// describeInterest renders an interest in one compact line. Only the axes that
// were declared appear, so the line stays short enough to be worth printing in
// a status block an agent reads on every turn.
func describeInterest(i activity.Interest) string {
	var parts []string
	add := func(label string, values []string) {
		if len(values) > 0 {
			parts = append(parts, label+"="+strings.Join(values, ","))
		}
	}
	add("source", i.Sources)
	add("noun", i.Nouns)
	add("action", i.Actions)
	add("repo", i.Repos)
	add("sprint", i.Sprints)
	add("topic", i.Topics)
	add("object", i.Objects)
	if i.Audit {
		parts = append(parts, "audit")
	}
	if i.MaxWakePerMin > 0 {
		parts = append(parts, "max-wake-per-min="+strconv.Itoa(i.MaxWakePerMin))
	}
	if len(parts) == 0 {
		return "(no axis declared — matches only mention/assignment/ownership)"
	}
	return strings.Join(parts, " ")
}

func activityTail(w io.Writer, args []string) int {
	f, err := parseActivityFlags(args, []string{"source"}, []string{"json"}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy activity tail: %v\n", err)
		return 2
	}
	limit := 20
	if len(f.rest) > 0 {
		if n, cerr := strconv.Atoi(f.rest[0]); cerr == nil && n > 0 {
			limit = n
		}
	}
	source := ""
	if got := f.lists["source"]; len(got) > 0 {
		source = got[0]
	}
	records, err := activity.Tail(limit, source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy activity tail: %v\n", err)
		return 1
	}
	if f.bools["json"] {
		enc := json.NewEncoder(w)
		for _, r := range records {
			if err := enc.Encode(r); err != nil {
				return 1
			}
		}
		return 0
	}
	// The compact default: exactly the subject a recipient saw, plus the
	// delivery count. One line per event, no header, nothing an agent has to
	// pay tokens to skip.
	for _, r := range records {
		fmt.Fprintf(w, "%s -> %d\n", r.Event.Subject(), len(r.Recipients))
	}
	if len(records) == 0 {
		fmt.Fprintln(w, "no activity recorded")
	}
	return 0
}

func activityShow(w io.Writer, args []string) int {
	f, _ := parseActivityFlags(args, nil, []string{"json"}, nil)
	if len(f.rest) != 1 {
		fmt.Fprintln(os.Stderr, "bashy activity show: name exactly one event id")
		return 2
	}
	rec, ok, err := activity.Show(f.rest[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy activity show: %v\n", err)
		return 1
	}
	if !ok {
		// An honest miss. The journal is pruned, so an id can outlive its
		// record — saying "not found" without saying why would look like the
		// event never existed.
		fmt.Fprintf(os.Stderr, "bashy activity show: no record for %q (it may have been pruned; the journal keeps the newest %d)\n",
			f.rest[0], activity.MaxJournalRecords)
		return 1
	}
	if f.bools["json"] {
		return emitJSON(w, rec)
	}
	return emitJSON(w, rec.Event)
}

func activityRoutes(w io.Writer, args []string) int {
	f, _ := parseActivityFlags(args, nil, []string{"json"}, nil)
	if len(f.rest) != 1 {
		fmt.Fprintln(os.Stderr, "bashy activity routes: name exactly one event id")
		return 2
	}
	rec, ok, err := activity.Show(f.rest[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy activity routes: %v\n", err)
		return 1
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "bashy activity routes: no record for %q\n", f.rest[0])
		return 1
	}
	if f.bools["json"] {
		return emitJSON(w, rec.Recipients)
	}
	if len(rec.Recipients) == 0 {
		fmt.Fprintln(w, "routed to nobody (no named relationship connected this event to any identity)")
		return 0
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SUBSCRIBER\tREASON\tWAKE\tWHY")
	for _, r := range rec.Recipients {
		outcome := rec.Wakes[r.Subscriber]
		if outcome == "" {
			outcome = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Subscriber, r.Reason, outcome, r.Why)
	}
	return flushTab(tw)
}

func activityRecover(w io.Writer, args []string) int {
	f, _ := parseActivityFlags(args, nil, []string{"json"}, nil)
	out, err := activity.Recover()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy activity recover: %v\n", err)
		return 1
	}
	if f.bools["json"] {
		return emitJSON(w, out)
	}
	if len(out) == 0 {
		fmt.Fprintln(w, "nothing owed")
		return 0
	}
	for _, r := range out {
		fmt.Fprintf(w, "%s -> %d recipient(s)", r.Event.Subject(), len(r.Recipients))
		if len(r.Errors) > 0 {
			fmt.Fprintf(w, " (%s)", strings.Join(r.Errors, "; "))
		}
		fmt.Fprintln(w)
	}
	return 0
}

func activityStatus(w io.Writer, args []string) int {
	f, _ := parseActivityFlags(args, nil, []string{"json"}, nil)
	records, err := activity.Tail(0, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy activity status: %v\n", err)
		return 1
	}
	interests, _ := activity.LoadInterests()

	type report struct {
		Schema    string           `json:"schema"`
		Dir       string           `json:"dir"`
		Events    int              `json:"events"`
		Owed      int              `json:"owed"`
		Interests int              `json:"interests"`
		HighSeq   map[string]int64 `json:"high_seq,omitempty"`
		Wakes     map[string]int   `json:"wakes,omitempty"`
	}
	rep := report{
		Schema: activity.SchemaVersion, Dir: activity.StateDir(),
		Events: len(records), Interests: len(interests),
		HighSeq: map[string]int64{}, Wakes: map[string]int{},
	}
	for _, r := range records {
		if !r.Published {
			rep.Owed++
		}
		if r.Event.Seq > rep.HighSeq[r.Event.Source] {
			rep.HighSeq[r.Event.Source] = r.Event.Seq
		}
		for _, outcome := range r.Wakes {
			rep.Wakes[outcome]++
		}
	}
	if f.bools["json"] {
		return emitJSON(w, rep)
	}
	fmt.Fprintf(w, "schema:    %s\n", rep.Schema)
	fmt.Fprintf(w, "outbox:    %s\n", emptyAs(rep.Dir, "(none — set BASHY_ACTIVITY_DIR or BASHY_HOME)"))
	fmt.Fprintf(w, "events:    %d\n", rep.Events)
	fmt.Fprintf(w, "owed:      %d", rep.Owed)
	if rep.Owed > 0 {
		// An owed delivery is not a lost one — that is the whole point of
		// journaling before publishing — so say what closes it.
		fmt.Fprint(w, "  (run `bashy activity recover`)")
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "interests: %d\n", rep.Interests)
	if len(rep.HighSeq) > 0 {
		var parts []string
		for _, s := range activity.Sources() {
			if seq, ok := rep.HighSeq[s]; ok {
				parts = append(parts, fmt.Sprintf("%s=%d", s, seq))
			}
		}
		fmt.Fprintf(w, "sequence:  %s\n", strings.Join(parts, " "))
	}
	if len(rep.Wakes) > 0 {
		var parts []string
		for _, k := range []string{activity.WakeSteered, activity.WakeQueued, activity.WakeCoalesced, activity.WakeRateLimited, activity.WakeUnreachable} {
			if n, ok := rep.Wakes[k]; ok {
				parts = append(parts, fmt.Sprintf("%s=%d", k, n))
			}
		}
		fmt.Fprintf(w, "wakes:     %s\n", strings.Join(parts, " "))
	}
	return 0
}

func emitJSON(w io.Writer, v any) int {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return 1
	}
	return 0
}

func flushTab(tw *tabwriter.Writer) int {
	if err := tw.Flush(); err != nil {
		return 1
	}
	return 0
}
