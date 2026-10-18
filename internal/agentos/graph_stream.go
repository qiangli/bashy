// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/qiangli/yoke/pkg/lens/provider"
	"github.com/qiangli/yoke/pkg/lens/stream"
	"github.com/qiangli/yoke/pkg/ref"
)

// Sprint 446 S19: the coordination nouns' list/show verbs emit the shared
// bashy-graph-v1 stream under --json, so `bashy sprint show 446 --json |
// bashy lens -t tree` works, and the verbs that act on things take addresses
// back in through a literal `-` operand. The commands themselves live in
// yoke; the front door projects their native --json document on the way out,
// the same seam wrapFleetListEnvelope uses, so classic text is untouched and
// a failed or unrecognised document passes through byte-for-byte.
//
// A projection keeps declared, stable fields only: wall-clock-relative values
// (age, overdue, live resources, heartbeats) are dropped, so one snapshot
// gives one byte sequence and stream.Encode's content rev is meaningful.
// Edges may point at an entity the stream does not carry (an assignee, an
// owner); its address resolves through its own command.

// graphProjection turns one command's native --json bytes into a stream
// graph. errNotProjectable leaves the native bytes in place.
type graphProjection func(raw []byte) (*stream.Graph, error)

var errNotProjectable = errors.New("graph stream: document is not projectable")

// graphProjections is keyed by canonical command path (graphCommandPath).
var graphProjections = map[string]graphProjection{
	"sprint board": projectSprintBoard,
	"sprint show":  projectSprintShow,
	"todo list":    projectTodoList,
	"todo show":    projectTodoShow,
	"agent list":   projectAgentList,
	"agent show":   projectAgentShow,
	"claim list":   projectClaimList,
	"mb":           projectMBPosts,
	"mb show":      projectMBPosts,
	"meet show":    projectMeetShow,
}

// addressConsumers name the operand a literal `-` may replace, per path.
var addressConsumers = map[string]addressConsumer{
	"sprint claim": {operand: 1, flag: "", normalize: storyAddress},
	"mb send":      {operand: 0, flag: "to", normalize: recipientAddress},
}

// graphCommandPath folds the front door's alias spellings (`issue`,
// `agents`, `messages`) onto the canonical noun so one table serves all.
func graphCommandPath(c *cobra.Command) string {
	fields := strings.Fields(c.CommandPath())
	if len(fields) == 0 {
		return ""
	}
	switch fields[0] {
	case "issue":
		fields[0] = "todo"
	case "messages":
		fields[0] = "mb"
	default:
		fields[0] = fleetNoun(fields[0])
	}
	return strings.Join(fields, " ")
}

// installGraphStreams arranges the S19 behaviour on the resolved target.
func installGraphStreams(target *cobra.Command) {
	path := graphCommandPath(target)
	if p, ok := graphProjections[path]; ok && target.Flags().Lookup("json") != nil {
		wrapRun(target, func(inner func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
			return graphJSONRun(path, p, inner)
		})
	}
	if a, ok := addressConsumers[path]; ok {
		wrapRun(target, func(inner func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
			return a.run(path, inner)
		})
	}
}

func graphJSONRun(path string, project graphProjection, inner func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(c *cobra.Command, args []string) error {
		if f := c.Flags().Lookup("json"); f == nil || !f.Changed || f.Value.String() != "true" {
			return inner(c, args)
		}
		real := c.OutOrStdout()
		var buf bytes.Buffer
		c.SetOut(&buf)
		runErr := inner(c, args)
		c.SetOut(real)
		if runErr == nil {
			if g, err := project(buf.Bytes()); err == nil {
				return stream.Encode(real, g, graphProvider(path), graphQuery(path, c, args))
			} else if !errors.Is(err, errNotProjectable) {
				return err
			}
		}
		_, err := real.Write(buf.Bytes())
		if runErr != nil {
			return runErr
		}
		return err
	}
}

// graphProvider names the issuing provider in the cursor binding: the noun.
func graphProvider(path string) string {
	noun, _, _ := strings.Cut(path, " ")
	return noun
}

// graphQuery is the cursor binding's query: the verb, its operands and every
// flag the caller set, in pflag's sorted visit order.
func graphQuery(path string, c *cobra.Command, args []string) string {
	q := url.Values{"op": {path}}
	for _, a := range args {
		q.Add("arg", a)
	}
	c.Flags().Visit(func(f *pflag.Flag) {
		if f.Name != "json" {
			q.Add("flag", f.Name+"="+f.Value.String())
		}
	})
	return q.Encode()
}

// graphBuilder collects nodes (first ref wins) and distinct edges.
type graphBuilder struct {
	g     stream.Graph
	nodes map[string]bool
	edges map[[3]string]bool
}

func newGraphBuilder(source string) *graphBuilder {
	return &graphBuilder{g: stream.Graph{Source: source}, nodes: map[string]bool{}, edges: map[[3]string]bool{}}
}

func (b *graphBuilder) node(ref, name, kind, typ string, attrs map[string]any) {
	if ref == "" || b.nodes[ref] {
		return
	}
	if name == "" {
		name = ref
	}
	b.nodes[ref] = true
	b.g.Nodes = append(b.g.Nodes, provider.Node{SchemaVersion: provider.NodeSchema, Ref: ref, Name: name, Kind: kind, Type: typ, Attrs: attrs})
}

func (b *graphBuilder) edge(src, rel, dst string, attrs map[string]any) {
	k := [3]string{src, rel, dst}
	if src == "" || dst == "" || b.edges[k] {
		return
	}
	if attrs == nil {
		attrs = map[string]any{}
	}
	b.edges[k] = true
	b.g.Edges = append(b.g.Edges, provider.Edge{SchemaVersion: provider.EdgeSchema, Src: src, Rel: rel, Dst: dst, Attrs: attrs})
}

func (b *graphBuilder) graph() (*stream.Graph, error) { return &b.g, nil }

type jsonObj = map[string]any

// decodeDoc decodes one JSON document with numbers kept verbatim, unwrapping
// the loom-v2 envelope's result; an error envelope is not projectable.
func decodeDoc(raw []byte) (any, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, errNotProjectable
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errNotProjectable
	}
	if o, ok := v.(jsonObj); ok && o["schema_version"] == "loom-v2" {
		if o["status"] != "ok" {
			return nil, errNotProjectable
		}
		return o["result"], nil
	}
	return v, nil
}

func decodeObj(raw []byte) (jsonObj, error) {
	v, err := decodeDoc(raw)
	if err != nil {
		return nil, err
	}
	o, ok := v.(jsonObj)
	if !ok {
		return nil, errNotProjectable
	}
	return o, nil
}

// pick copies the named fields that are present and non-empty.
func pick(o jsonObj, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		v, ok := o[k]
		if !ok || v == nil || v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func str(o jsonObj, k string) string {
	switch v := o[k].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	}
	return ""
}

func objs(v any) []jsonObj {
	list, _ := v.([]any)
	out := make([]jsonObj, 0, len(list))
	for _, e := range list {
		if o, ok := e.(jsonObj); ok {
			out = append(out, o)
		}
	}
	return out
}

// principalRef addresses an owner/assignee/sender: an existing ref is kept,
// a bare name is an agent.
func principalRef(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if r, err := ref.Parse(name); err == nil && strings.Contains(name, ":") {
		return r.String()
	}
	return ref.Format(ref.Agent, name)
}

func collectionSource(kind ref.Kind, target string) string {
	return ref.Locator{Scheme: string(kind), Target: target}.String()
}

var sprintCardFields = []string{"id", "uuid", "slug", "title", "epic", "column", "owner", "spec_ref", "created", "updated_at"}

func sprintCard(b *graphBuilder, s jsonObj, extra ...string) string {
	id := str(s, "id")
	if id == "" {
		return ""
	}
	r := ref.Format(ref.Sprint, id)
	b.node(r, str(s, "title"), "sprint", "sprint-card", pick(s, append(sprintCardFields, extra...)...))
	b.edge(r, "owned-by", principalRef(str(s, "owner")), nil)
	for _, run := range objs(s["runs"]) {
		repo, n := str(run, "repo"), str(run, "id")
		if repo == "" || n == "" {
			continue
		}
		rr := ref.Format(ref.Run, repo+"-"+n)
		b.edge(r, "links-run", rr, nil)
	}
	return r
}

// projectSprintBoard is the sprint list: one node per card.
func projectSprintBoard(raw []byte) (*stream.Graph, error) {
	res, err := decodeObj(raw)
	if err != nil {
		return nil, err
	}
	cards, ok := res["stories"]
	if !ok {
		return nil, errNotProjectable
	}
	b := newGraphBuilder(collectionSource(ref.Sprint, "*"))
	for _, s := range objs(cards) {
		sprintCard(b, s)
	}
	return b.graph()
}

// projectSprintShow is one card as a tree: sprint contains goals, a goal
// contains its stories; runs, the next story and the owner are edges.
func projectSprintShow(raw []byte) (*stream.Graph, error) {
	res, err := decodeObj(raw)
	if err != nil {
		return nil, err
	}
	s, ok := res["sprint"].(jsonObj)
	if !ok || str(s, "id") == "" {
		return nil, errNotProjectable
	}
	id := str(s, "id")
	b := newGraphBuilder(ref.Format(ref.Sprint, id))
	sr := sprintCard(b, s, "primary_goal", "acceptance", "continuity", "lease_epoch")
	progress := map[string]jsonObj{}
	for _, p := range objs(res["goal_progress"]) {
		progress[str(p, "id")] = p
	}
	story := func(st jsonObj) string {
		sid := str(st, "id")
		if sid == "" {
			return ""
		}
		r := ref.Format(ref.Todo, sid)
		b.node(r, firstNonEmpty(str(st, "title"), sid), "story", "todo", pick(st, "id", "repo", "title", "status", "priority", "seq"))
		return r
	}
	// The next story first so its richer record wins the node.
	if next, ok := res["next_story"].(jsonObj); ok {
		if rf, ok := next["ref"].(jsonObj); ok {
			st := pick(next, "title", "status", "priority", "seq")
			st["id"], st["repo"] = rf["id"], rf["repo"]
			b.edge(sr, "next", story(st), nil)
		}
	}
	for _, g := range objs(s["goal"]) {
		gid := str(g, "id")
		if gid == "" {
			continue
		}
		gr := ref.Locator{Scheme: string(ref.Sprint), Target: id, Inner: []string{gid}}.String()
		attrs := pick(g, "id", "text")
		for k, v := range pick(progress[gid], "checked", "wontfix") {
			attrs[k] = v
		}
		b.node(gr, firstNonEmpty(str(g, "text"), gid), "goal", "sprint-goal", attrs)
		b.edge(sr, "contains", gr, nil)
		for _, st := range objs(g["stories"]) {
			b.edge(gr, "contains", story(st), nil)
		}
	}
	for _, run := range objs(s["runs"]) {
		repo, n := str(run, "repo"), str(run, "id")
		if repo == "" || n == "" {
			continue
		}
		rr := ref.Format(ref.Run, repo+"-"+n)
		b.node(rr, repo+"-"+n, "run", "weave-run", pick(run, "repo", "id", "queue", "born", "story"))
		b.edge(sr, "contains", rr, nil)
		if st := str(run, "story"); st != "" {
			b.edge(rr, "works-on", ref.Format(ref.Todo, st), nil)
		}
	}
	return b.graph()
}

var todoFields = []string{"id", "seq", "title", "state", "status", "priority", "kind", "labels", "scope", "created", "due", "closed", "recurring", "assignee", "sprint", "sprint_id"}

func todoNode(b *graphBuilder, it jsonObj, extra ...string) string {
	id := str(it, "id")
	if id == "" {
		return ""
	}
	r := ref.Format(ref.Todo, id)
	b.node(r, firstNonEmpty(str(it, "title"), id), "todo", firstNonEmpty(str(it, "kind"), "task"), pick(it, append(todoFields, extra...)...))
	b.edge(r, "assigned-to", principalRef(str(it, "assignee")), nil)
	if sp := str(it, "sprint"); sp != "" && sp != "0" {
		b.edge(r, "story-of", ref.Format(ref.Sprint, sp), nil)
	}
	return r
}

func projectTodoList(raw []byte) (*stream.Graph, error) {
	res, err := decodeObj(raw)
	if err != nil {
		return nil, err
	}
	items, ok := res["items"]
	if !ok {
		return nil, errNotProjectable
	}
	b := newGraphBuilder(collectionSource(ref.Todo, firstNonEmpty(str(res, "folder"), str(res, "scope"), "*")))
	for _, it := range objs(items) {
		todoNode(b, it)
	}
	return b.graph()
}

func projectTodoShow(raw []byte) (*stream.Graph, error) {
	it, err := decodeObj(raw)
	if err != nil || str(it, "id") == "" {
		return nil, errNotProjectable
	}
	b := newGraphBuilder(ref.Format(ref.Todo, str(it, "id")))
	r := todoNode(b, it, "body", "links")
	for _, l := range objs(it["outbound"]) {
		b.edge(r, "links", str(l, "ref"), pick(l, "status"))
	}
	for _, l := range objs(it["inbound"]) {
		b.edge(str(l, "ref"), "links", r, pick(l, "status"))
	}
	return b.graph()
}

var agentFields = []string{"name", "nick", "auto_nick", "display", "band", "tool", "model", "binding", "kind", "billing", "provider", "reliability", "ring", "permission", "effort", "retired"}

func agentNode(b *graphBuilder, a jsonObj) {
	name := str(a, "name")
	if name == "" {
		return
	}
	r := ref.Format(ref.Agent, name)
	b.node(r, name, "agent", "fleet-agent", pick(a, agentFields...))
	if t := str(a, "tool"); t != "" {
		b.edge(r, "uses-tool", ref.Format(ref.Tool, t), nil)
	}
	if m := str(a, "model"); m != "" {
		b.edge(r, "uses-model", ref.Format(ref.Model, m), nil)
	}
}

func projectAgentList(raw []byte) (*stream.Graph, error) {
	res, err := decodeObj(raw)
	if err != nil {
		return nil, err
	}
	items, ok := res["items"]
	if !ok {
		return nil, errNotProjectable
	}
	b := newGraphBuilder(collectionSource(ref.Agent, "*"))
	for _, a := range objs(items) {
		agentNode(b, a)
	}
	return b.graph()
}

func projectAgentShow(raw []byte) (*stream.Graph, error) {
	a, err := decodeObj(raw)
	if err != nil || str(a, "name") == "" {
		return nil, errNotProjectable
	}
	b := newGraphBuilder(ref.Format(ref.Agent, str(a, "name")))
	agentNode(b, a)
	return b.graph()
}

// projectClaimList addresses a claim as claim:KIND!/KEY, KEY being the
// resource it names (or the project for a repo claim).
func projectClaimList(raw []byte) (*stream.Graph, error) {
	v, err := decodeDoc(raw)
	if err != nil {
		return nil, err
	}
	if _, ok := v.([]any); !ok {
		return nil, errNotProjectable
	}
	b := newGraphBuilder(ref.Locator{Scheme: "claim", Target: "*"}.String())
	for _, c := range objs(v) {
		kind, key := str(c, "kind"), firstNonEmpty(str(c, "resource"), str(c, "project"))
		if kind == "" || key == "" {
			continue
		}
		r := ref.Locator{Scheme: "claim", Target: kind, Inner: []string{key}}.String()
		attrs := pick(c, "kind", "project", "resource", "mode", "intent", "epoch", "rev", "acquired_at", "members")
		holder, _ := c["holder"].(jsonObj)
		if h := str(holder, "name"); h != "" {
			attrs["holder"] = h
		}
		b.node(r, key, "claim", kind, attrs)
		hk := ref.Kind(str(holder, "kind"))
		if !ref.Known(hk) {
			hk = ref.Agent
		}
		if h := str(holder, "name"); h != "" {
			b.edge(r, "held-by", ref.Format(hk, h), nil)
		}
	}
	return b.graph()
}

// projectMBPosts covers `mb --json` (one post per line, or the typed empty
// line) and `mb show --json` (one indented post).
func projectMBPosts(raw []byte) (*stream.Graph, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var posts []jsonObj
	for {
		var o jsonObj
		if err := dec.Decode(&o); err == io.EOF {
			break
		} else if err != nil {
			return nil, errNotProjectable
		}
		if _, empty := o["posts"]; empty {
			continue
		}
		if str(o, "seq") == "" {
			return nil, errNotProjectable
		}
		posts = append(posts, o)
	}
	source := collectionSource(ref.MB, "*")
	if len(posts) == 1 && str(posts[0], "ref") != "" {
		source = str(posts[0], "ref")
	}
	b := newGraphBuilder(source)
	for _, p := range posts {
		r := ref.Format(ref.MB, str(p, "seq"))
		name := strings.TrimSpace(str(p, "body"))
		if i := strings.IndexByte(name, '\n'); i >= 0 {
			name = name[:i]
		}
		if len(name) > 72 {
			name = name[:72]
		}
		b.node(r, firstNonEmpty(name, r), "post", "mb-post", pick(p, "id", "seq", "at", "from", "to", "audience", "mode", "topic", "body"))
		b.edge(r, "from", principalRef(str(p, "from")), nil)
		b.edge(r, "to", principalRef(str(p, "to")), nil)
	}
	return b.graph()
}

// projectMeetShow addresses the meeting by its durable id (never the room
// number); the agenda is contained, participants are edges carrying their
// coverage counts.
func projectMeetShow(raw []byte) (*stream.Graph, error) {
	o, err := decodeObj(raw)
	if err != nil {
		return nil, err
	}
	st, ok := o["state"].(jsonObj)
	if !ok || str(st, "id") == "" {
		return nil, errNotProjectable
	}
	id := str(st, "id")
	r := ref.Format(ref.Meet, id)
	b := newGraphBuilder(r)
	b.node(r, firstNonEmpty(str(st, "topic"), id), "meeting", "meet-room",
		pick(st, "id", "room", "topic", "status", "human", "initiator", "secretary", "created", "round", "decision_mode", "out"))
	agenda, _ := st["agenda"].([]any)
	for i, a := range agenda {
		text, _ := a.(string)
		if text == "" {
			continue
		}
		ar := ref.Locator{Scheme: string(ref.Meet), Target: id, Inner: []string{"agenda-" + strconv.Itoa(i+1)}}.String()
		b.node(ar, text, "agenda-item", "meet-agenda", map[string]any{"index": i + 1, "text": text})
		b.edge(r, "contains", ar, nil)
	}
	coverage := map[string]jsonObj{}
	for _, c := range objs(o["coverage"]) {
		coverage[str(c, "name")] = c
	}
	participants, _ := st["participants"].([]any)
	for _, p := range participants {
		name, _ := p.(string)
		if po, ok := p.(jsonObj); ok {
			name = str(po, "name")
		}
		if name == "" {
			continue
		}
		b.edge(r, "participant", principalRef(name), pick(coverage[name], "role", "turns", "ok", "errors"))
	}
	return b.graph()
}

// addressConsumer replaces a literal `-` (the operand at index operand, or
// the value of flag) with each address read from stdin — a bashy-graph-v1
// stream (its node refs) or plain addresses one per line — running the verb
// once per address. Every address is validated before the first run, so a
// stream carrying something else acts on nothing.
type addressConsumer struct {
	operand   int
	flag      string
	normalize func(string) (string, error)
}

func (a addressConsumer) run(path string, inner func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(c *cobra.Command, args []string) error {
		viaFlag := false
		if a.flag != "" {
			if f := c.Flags().Lookup(a.flag); f != nil && f.Changed && f.Value.String() == "-" {
				viaFlag = true
			}
		}
		if !viaFlag && (len(args) <= a.operand || args[a.operand] != "-") {
			return inner(c, args)
		}
		raw, err := stream.ReadAddresses(c.InOrStdin())
		if err != nil {
			return fmt.Errorf("%s -: %w", path, err)
		}
		if len(raw) == 0 {
			return fmt.Errorf("%s -: no addresses on stdin", path)
		}
		addrs := make([]string, 0, len(raw))
		for _, r := range raw {
			v, err := a.normalize(r)
			if err != nil {
				return fmt.Errorf("%s -: %w", path, err)
			}
			addrs = append(addrs, v)
		}
		for _, v := range addrs {
			argv := append([]string(nil), args...)
			if viaFlag {
				if err := c.Flags().Set(a.flag, v); err != nil {
					return err
				}
			} else {
				argv[a.operand] = v
			}
			if err := inner(c, argv); err != nil {
				return err
			}
		}
		return nil
	}
}

// storyAddress accepts todo:ID, story:ID or a bare story id.
func storyAddress(s string) (string, error) {
	if !strings.Contains(s, ":") {
		return s, nil
	}
	if r, err := ref.Parse(s); err == nil && r.Kind == ref.Todo {
		return r.ID, nil
	}
	if l, err := ref.ParseLocator(s); err == nil && l.Scheme == "story" && len(l.Inner) == 0 && l.ScopeKind == "" {
		return l.Target, nil
	}
	return "", fmt.Errorf("%q is not a story address (todo:ID, story:ID or a bare id)", s)
}

// recipientAddress accepts agent:NAME, person:NAME or a bare name; the name
// is resolved by `mb send <agent>` at send time, as on the command line.
func recipientAddress(s string) (string, error) {
	if !strings.Contains(s, ":") {
		return s, nil
	}
	if r, err := ref.Parse(s); err == nil {
		switch r.Kind {
		case ref.Agent, ref.Person:
			return r.ID, nil
		}
	}
	return "", fmt.Errorf("%q is not a recipient address (agent:NAME, person:NAME or a bare name)", s)
}
