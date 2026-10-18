// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/pkg/issue"
	"github.com/qiangli/yoke/pkg/lens/provider"
	"github.com/qiangli/yoke/pkg/lens/stream"
	"github.com/qiangli/yoke/pkg/todo"
)

// Native --json documents in the shapes the yoke verbs emit today, trimmed to
// the fields the projections read plus the volatile ones they must drop.
var graphStreamFixtures = map[string]struct {
	doc       string
	source    string
	nodes     []string
	edges     []string // "src rel dst"
	forbidden []string // volatile fields that must not reach the stream
}{
	"sprint show": {
		doc: `{"schema_version":"loom-v2","command":"sprint show","status":"ok","result":{
		  "goal_progress":[{"id":"s1","checked":true,"wontfix":false},{"id":"s2","checked":false,"wontfix":false}],
		  "next_story":{"ref":{"repo":"/r","id":"bbbbbbbbbbbb"},"title":"Second","status":"assigned","priority":"p0","seq":2},
		  "resources":{"at":"2026-10-18T10:50:54Z","cpu_percent":94.7},
		  "outcome_cost":{"elapsed":30737305713000},
		  "sprint":{"id":446,"uuid":"u-446","title":"Pipes","column":"doing","owner":"s446-manager",
		    "goal":[{"id":"s2","text":"Two","stories":[{"repo":"/r","id":"bbbbbbbbbbbb"}]},
		            {"id":"s1","text":"One","stories":[{"repo":"/r","id":"aaaaaaaaaaaa"}]}],
		    "runs":[{"repo":"bashy","id":7,"queue":"q","born":"2026-10-18T02:19:54Z","story":"aaaaaaaaaaaa"}],
		    "lease":{"holder":"s446-manager","at":"2026-10-18T10:50:04Z"},
		    "thread":[{"at":"2026-10-15T17:37:05Z","body":"created"}]}}}`,
		source: "sprint:446",
		nodes:  []string{"run:bashy-7", "sprint:446", "sprint:446!/s1", "sprint:446!/s2", "todo:aaaaaaaaaaaa", "todo:bbbbbbbbbbbb"},
		edges: []string{
			"run:bashy-7 works-on todo:aaaaaaaaaaaa",
			"sprint:446 contains run:bashy-7",
			"sprint:446 contains sprint:446!/s1",
			"sprint:446 contains sprint:446!/s2",
			"sprint:446 links-run run:bashy-7",
			"sprint:446 next todo:bbbbbbbbbbbb",
			"sprint:446 owned-by agent:s446-manager",
			"sprint:446!/s1 contains todo:aaaaaaaaaaaa",
			"sprint:446!/s2 contains todo:bbbbbbbbbbbb",
		},
		forbidden: []string{"cpu_percent", "elapsed", "thread", "token_hash"},
	},
	"sprint board": {
		doc: `{"schema_version":"loom-v2","command":"sprint board","status":"ok","result":{"stories":[
		  {"id":2,"title":"B","column":"done","owner":"x","thread":[{"at":"t","body":"b"}]},
		  {"id":1,"title":"A","column":"doing","runs":[{"repo":"yoke","id":3}]}]}}`,
		source:    "sprint:*",
		nodes:     []string{"sprint:1", "sprint:2"},
		edges:     []string{"sprint:1 links-run run:yoke-3", "sprint:2 owned-by agent:x"},
		forbidden: []string{"thread"},
	},
	"todo list": {
		doc: `{"schema_version":"loom-v2","command":"todo list","status":"ok","result":{"scope":"repo /r","folder":"/r/docs/todo","count":2,"items":[
		  {"id":"bbbbbbbbbbbb","seq":2,"title":"Two","state":"todo","age":"3d","sprint":446},
		  {"id":"aaaaaaaaaaaa","seq":1,"title":"One","state":"doing","age":"93d","overdue":true,"assignee":"codex"}]}}`,
		source:    "todo:/r/docs/todo",
		nodes:     []string{"todo:aaaaaaaaaaaa", "todo:bbbbbbbbbbbb"},
		edges:     []string{"todo:aaaaaaaaaaaa assigned-to agent:codex", "todo:bbbbbbbbbbbb story-of sprint:446"},
		forbidden: []string{`"age"`, "overdue"},
	},
	"todo show": {
		doc: `{"id":"aaaaaaaaaaaa","kind":"bug","title":"One","seq":1,"status":"doing","assignee":"person:qiangli","body":"text",
		  "ref":"todo:aaaaaaaaaaaa","overdue":false,
		  "outbound":[{"ref":"kb:lesson","status":"resolved"}],"inbound":[{"ref":"todo:cccccccccccc","status":"resolved"}]}`,
		source: "todo:aaaaaaaaaaaa",
		nodes:  []string{"todo:aaaaaaaaaaaa"},
		edges: []string{
			"todo:aaaaaaaaaaaa assigned-to person:qiangli",
			"todo:aaaaaaaaaaaa links kb:lesson",
			"todo:cccccccccccc links todo:aaaaaaaaaaaa",
		},
		forbidden: []string{"overdue"},
	},
	"agent list": {
		doc: `{"schema_version":"bashy-registry-list-v1","kind":"agent","view":"default","items":[
		  {"name":"codex-gpt6-sol","tool":"codex","model":"gpt6-sol","band":4},{"name":"claude-opus5","tool":"claude","model":"opus5","band":4}]}`,
		source: "agent:*",
		nodes:  []string{"agent:claude-opus5", "agent:codex-gpt6-sol"},
		edges: []string{
			"agent:claude-opus5 uses-model model:opus5", "agent:claude-opus5 uses-tool tool:claude",
			"agent:codex-gpt6-sol uses-model model:gpt6-sol", "agent:codex-gpt6-sol uses-tool tool:codex",
		},
	},
	"agent show": {
		doc:    `{"name":"claude-opus5","display":"claude · opus5","tool":"claude","model":"opus5","ledger":{"notes":"n"},"ring":2}`,
		source: "agent:claude-opus5",
		nodes:  []string{"agent:claude-opus5"},
		edges:  []string{"agent:claude-opus5 uses-model model:opus5", "agent:claude-opus5 uses-tool tool:claude"},
	},
	"claim list": {
		doc: `[{"schema_version":"bashy-claim-v2","kind":"repo","project":"yoke","epoch":7,"rev":30,
		   "holder":{"kind":"agent","name":"s457w2-manager"},"heartbeat":"2026-10-18T10:49:31Z","pid":1},
		  {"schema_version":"bashy-claim-v2","kind":"name","resource":"vsc-arm","mode":"lease",
		   "holder":{"kind":"person","name":"qiangli"},"heartbeat":"2026-10-18T10:46:36Z"}]`,
		source:    "claim:*",
		nodes:     []string{"claim:name!/vsc-arm", "claim:repo!/yoke"},
		edges:     []string{"claim:name!/vsc-arm held-by person:qiangli", "claim:repo!/yoke held-by agent:s457w2-manager"},
		forbidden: []string{"heartbeat"},
	},
	"mb": {
		doc: `{"schema_version":"bashy-mb-v1","seq":2,"at":"2026-08-03T16:48:00Z","from":"qiangli","topic":"announce","body":"wall post"}
{"schema_version":"bashy-mb-v1","seq":1,"at":"2026-08-03T16:47:52Z","from":"qiangli","to":"codex-gpt5.6-sol","topic":"mb","body":"check the disk\nbefore your next build"}
`,
		source: "mb:*",
		nodes:  []string{"mb:1", "mb:2"},
		edges:  []string{"mb:1 from agent:qiangli", "mb:1 to agent:codex-gpt5.6-sol", "mb:2 from agent:qiangli"},
	},
	"mb show": {
		doc: `{
  "ref": "mb:1",
  "schema_version": "bashy-mb-v1",
  "seq": 1,
  "from": "qiangli",
  "to": "codex-gpt5.6-sol",
  "body": "check the disk"
}`,
		source: "mb:1",
		nodes:  []string{"mb:1"},
		edges:  []string{"mb:1 from agent:qiangli", "mb:1 to agent:codex-gpt5.6-sol"},
	},
	"meet show": {
		doc: `{"coverage":[{"name":"claude-opus4.8","role":"participant","turns":2,"ok":1,"errors":1,"chars":1528,"last_status":"error"}],
		  "links":null,"synthesis":null,
		  "state":{"schema":"bashy-meet-v1","id":"2026-08-27-audit-2ca9","room":1,"topic":"Audit","agenda":["scope","verdict"],
		    "participants":["claude-opus4.8","codex-gpt6-sol"],"status":"open","cwd":"/home/x","turn_timeout":"20m"}}`,
		source: "meet:2026-08-27-audit-2ca9",
		nodes:  []string{"meet:2026-08-27-audit-2ca9", "meet:2026-08-27-audit-2ca9!/agenda-1", "meet:2026-08-27-audit-2ca9!/agenda-2"},
		edges: []string{
			"meet:2026-08-27-audit-2ca9 contains meet:2026-08-27-audit-2ca9!/agenda-1",
			"meet:2026-08-27-audit-2ca9 contains meet:2026-08-27-audit-2ca9!/agenda-2",
			"meet:2026-08-27-audit-2ca9 participant agent:claude-opus4.8",
			"meet:2026-08-27-audit-2ca9 participant agent:codex-gpt6-sol",
		},
		forbidden: []string{"chars", "last_status", "turn_timeout"},
	},
}

func encodeProjection(t *testing.T, path, doc string) string {
	t.Helper()
	g, err := graphProjections[path]([]byte(doc))
	if err != nil {
		t.Fatalf("%s: project: %v", path, err)
	}
	var b bytes.Buffer
	if err := stream.Encode(&b, g, graphProvider(path), "op="+path); err != nil {
		t.Fatalf("%s: encode: %v", path, err)
	}
	return b.String()
}

// Every declared projection emits a complete, self-describing frame: the
// header first, nodes sorted by ref, edges by (src, rel, dst), and a cursor
// trailer bound to the source and the content rev — and the same document
// always gives the same bytes.
func TestGraphStreamProjectionsFrameDeterministically(t *testing.T) {
	for path := range graphProjections {
		if _, ok := graphStreamFixtures[path]; !ok {
			t.Errorf("projection %q has no fixture", path)
		}
	}
	for path, fx := range graphStreamFixtures {
		t.Run(strings.ReplaceAll(path, " ", "_"), func(t *testing.T) {
			out := encodeProjection(t, path, fx.doc)
			if again := encodeProjection(t, path, fx.doc); again != out {
				t.Fatalf("not byte-deterministic:\n%s\n---\n%s", out, again)
			}
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			if !strings.HasPrefix(lines[0], `{"t":"header","schema_version":"bashy-graph-v1","source":"`+fx.source+`","rev":"sha256:`) {
				t.Errorf("header: %s", lines[0])
			}
			if !strings.HasPrefix(lines[len(lines)-1], `{"t":"cursor","cursor":{"schema_version":"bashy-cursor-v1","kind":"pagination"`) {
				t.Errorf("trailer: %s", lines[len(lines)-1])
			}
			g, err := stream.Decode(strings.NewReader(out), stream.DecodeOptions{})
			if err != nil {
				t.Fatalf("undeclared decode refuses the frame: %v\n%s", err, out)
			}
			if err := g.Verify(); err != nil {
				t.Fatal(err)
			}
			if b := g.Cursor.Binding; b.Provider != graphProvider(path) || b.Target != fx.source || b.Revision != g.Rev {
				t.Errorf("cursor binding %+v, want provider %q target %q rev %q", b, graphProvider(path), fx.source, g.Rev)
			}
			var refs, edges []string
			for _, n := range g.Nodes {
				refs = append(refs, n.Ref)
			}
			for _, e := range g.Edges {
				edges = append(edges, e.Src+" "+e.Rel+" "+e.Dst)
			}
			if got, want := strings.Join(refs, "\n"), strings.Join(fx.nodes, "\n"); got != want {
				t.Errorf("nodes:\n%s\nwant:\n%s", got, want)
			}
			if got, want := strings.Join(edges, "\n"), strings.Join(fx.edges, "\n"); got != want {
				t.Errorf("edges:\n%s\nwant:\n%s", got, want)
			}
			for _, f := range fx.forbidden {
				if strings.Contains(out, f) {
					t.Errorf("volatile field %s reached the stream:\n%s", f, out)
				}
			}
		})
	}
}

// Producer order never leaks: a list whose items arrive in another order
// encodes to the same bytes (same rev, same cursor).
func TestGraphStreamListOrderIndependent(t *testing.T) {
	a := `{"schema_version":"loom-v2","status":"ok","result":{"folder":"/r","items":[{"id":"aaaaaaaaaaaa","title":"A"},{"id":"bbbbbbbbbbbb","title":"B"}]}}`
	b := `{"schema_version":"loom-v2","status":"ok","result":{"folder":"/r","items":[{"id":"bbbbbbbbbbbb","title":"B"},{"id":"aaaaaaaaaaaa","title":"A"}]}}`
	if x, y := encodeProjection(t, "todo list", a), encodeProjection(t, "todo list", b); x != y {
		t.Fatalf("item order changed the stream:\n%s\n---\n%s", x, y)
	}
}

// fakeVerb builds ROOT SUB whose body writes doc under --json and text
// otherwise, the shape every wrapped yoke verb has.
func fakeVerb(root, sub, doc string, runErr error) *cobra.Command {
	r := &cobra.Command{Use: root, SilenceErrors: true, SilenceUsage: true}
	var jsonOut bool
	c := &cobra.Command{Use: sub, RunE: func(c *cobra.Command, _ []string) error {
		if jsonOut {
			fmt.Fprint(c.OutOrStdout(), doc)
		} else {
			fmt.Fprintln(c.OutOrStdout(), "classic text")
		}
		return runErr
	}}
	c.Flags().BoolVar(&jsonOut, "json", false, "")
	r.AddCommand(c)
	return r
}

func runVerb(t *testing.T, root *cobra.Command, stdin string, args ...string) (string, error) {
	t.Helper()
	out, err := applyCommonOptions(root, args)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(out)
	err = root.Execute()
	return buf.String(), err
}

// The front door projects --json on the way out, through the alias
// spelling too, and leaves classic text, error envelopes and failed runs
// byte-for-byte.
func TestGraphStreamFrontDoorWrapper(t *testing.T) {
	doc := graphStreamFixtures["todo list"].doc
	want := encodeProjection(t, "todo list", doc)
	got, err := runVerb(t, fakeVerb("issue", "list", doc, nil), "", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !stream.IsFramed([]byte(got)) || !strings.Contains(got, `"provider":"todo"`) {
		t.Fatalf("alias spelling not projected:\n%s", got)
	}
	if strings.Count(got, "\n") != strings.Count(want, "\n") {
		t.Fatalf("wrapper and projection disagree:\n%s\n---\n%s", got, want)
	}
	if got, _ := runVerb(t, fakeVerb("todo", "list", doc, nil), "", "list"); got != "classic text\n" {
		t.Errorf("classic text changed: %q", got)
	}
	errDoc := `{"schema_version":"loom-v2","command":"todo list","status":"error","error":{"code":"invalid_arg"}}`
	if got, _ := runVerb(t, fakeVerb("todo", "list", errDoc, nil), "", "list", "--json"); got != errDoc {
		t.Errorf("error envelope rewritten: %q", got)
	}
	boom := errors.New("boom")
	got, err = runVerb(t, fakeVerb("todo", "list", doc, boom), "", "list", "--json")
	if !errors.Is(err, boom) || got != doc {
		t.Errorf("failed run: err %v, output rewritten: %q", err, got)
	}
}

// The real yoke todo verb behind the front door: a repo list over
// --base-dir comes out as a frame whose nodes are the items, twice the same.
func TestGraphStreamTodoListEndToEnd(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs", "todo"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Seeded through the store, not `todo add`, so no ambient sprint
	// binding attaches the fixtures to a real card.
	for _, title := range []string{"first", "second"} {
		if _, err := todo.RepoStore(dir).Add(&issue.Issue{Kind: "task", Title: title, Status: "todo"}); err != nil {
			t.Fatal(err)
		}
	}
	list := func() string {
		out, err := runVerb(t, todo.NewTodoCmd(), "", "--base-dir", dir, "list", "--json")
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := list()
	g, err := stream.Decode(strings.NewReader(out), stream.DecodeOptions{})
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := g.Verify(); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, n := range g.Nodes {
		if n.Kind == "todo" {
			names = append(names, n.Name)
		}
	}
	if len(names) != 2 || !strings.HasPrefix(g.Source, "todo:") {
		t.Fatalf("source %q, todo nodes %v:\n%s", g.Source, names, out)
	}
	if again := list(); again != out {
		t.Fatalf("todo list --json is not byte-deterministic:\n%s\n---\n%s", out, again)
	}
}

func recordingVerb(root, sub string, seen *[][]string) *cobra.Command {
	r := &cobra.Command{Use: root, SilenceErrors: true, SilenceUsage: true}
	var to string
	c := &cobra.Command{Use: sub, Args: cobra.MinimumNArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		*seen = append(*seen, append([]string{"to=" + to}, args...))
		return nil
	}}
	c.Flags().StringVar(&to, "to", "", "")
	r.AddCommand(c)
	return r
}

func storyStream(t *testing.T, refs ...string) string {
	t.Helper()
	g := &stream.Graph{Source: "sprint:446"}
	for _, r := range refs {
		g.Nodes = append(g.Nodes, provider.Node{Ref: r, Name: r, Kind: "story", Type: "todo"})
	}
	var b bytes.Buffer
	if err := stream.Encode(&b, g, "sprint", "op=sprint+show"); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// `sprint claim SPRINT -` and `mb send - BODY` / `mb send --to - BODY`
// run once per address read from stdin: a stream's node refs or plain lines.
func TestAddressConsumersReadStdin(t *testing.T) {
	var seen [][]string
	in := storyStream(t, "todo:bbbbbbbbbbbb", "todo:aaaaaaaaaaaa")
	if _, err := runVerb(t, recordingVerb("sprint", "claim", &seen), in, "claim", "446", "-"); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(seen); got != "[[to= 446 aaaaaaaaaaaa] [to= 446 bbbbbbbbbbbb]]" {
		t.Errorf("sprint claim from a stream: %s", got)
	}

	seen = nil
	if _, err := runVerb(t, recordingVerb("sprint", "claim", &seen), "story:cccccccccccc\n\ndddddddddddd\n", "claim", "446", "-"); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(seen); got != "[[to= 446 cccccccccccc] [to= 446 dddddddddddd]]" {
		t.Errorf("sprint claim from plain locators: %s", got)
	}

	seen = nil
	if _, err := runVerb(t, recordingVerb("mb", "send", &seen), "agent:codex-gpt6-sol\nperson:qiangli\n", "send", "-", "gate", "red"); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(seen); got != "[[to= codex-gpt6-sol gate red] [to= qiangli gate red]]" {
		t.Errorf("mb send - : %s", got)
	}

	seen = nil
	if _, err := runVerb(t, recordingVerb("messages", "send", &seen), "claude-opus5\n", "send", "--to", "-", "hello"); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(seen); got != "[[to=claude-opus5 hello]]" {
		t.Errorf("mb send --to - : %s", got)
	}

	seen = nil
	if _, err := runVerb(t, recordingVerb("sprint", "claim", &seen), "", "claim", "446", "aaaaaaaaaaaa"); err != nil || fmt.Sprint(seen) != "[[to= 446 aaaaaaaaaaaa]]" {
		t.Errorf("a literal operand must pass through: %v %v", err, seen)
	}
}

// Every address is checked before the first run: a stream carrying anything
// but stories (the whole `sprint show` tree, say) claims nothing.
func TestAddressConsumersRefuseBeforeActing(t *testing.T) {
	cases := []struct{ path, stdin, want string }{
		{"claim", storyStream(t, "todo:aaaaaaaaaaaa", "sprint:446"), `"sprint:446" is not a story address`},
		{"claim", "", "no addresses on stdin"},
		{"send", "agent:x\nsprint:446\n", `"sprint:446" is not a recipient address`},
	}
	for _, c := range cases {
		var seen [][]string
		root := "sprint"
		args := []string{"claim", "446", "-"}
		if c.path == "send" {
			root, args = "mb", []string{"send", "-", "body"}
		}
		_, err := runVerb(t, recordingVerb(root, c.path, &seen), c.stdin, args...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %q: err %v, want %q", c.path, c.stdin, err, c.want)
		}
		if len(seen) != 0 {
			t.Errorf("%s acted before validating every address: %v", c.path, seen)
		}
	}
}
