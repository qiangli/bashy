// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Package adapter preserves bashy common-option, flag-normalization, and
// graph-stream semantics for standalone yoke command binaries invoked through
// their Bash# adapters.
package adapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/yoke/pkg/lens/provider"
	"github.com/qiangli/yoke/pkg/lens/stream"
	"github.com/qiangli/yoke/pkg/ref"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	startupInvokedAs = strings.TrimSpace(os.Getenv("BASHY_INVOKED_AS"))
	startupBashySelf = strings.TrimSpace(os.Getenv("BASHY_ADAPTER_SELF"))
)

func init() {
	StripAdapterEnv()
}

// StripAdapterEnv removes adapter-trampoline variables from the process
// environment so external CLIs launched by custom adapters never inherit them.
func StripAdapterEnv() {
	for _, k := range []string{
		"BASHY_SELF",
		"BASHY_ADAPTER_SELF",
		"BASHY_ADAPTER_EXEC",
		"BASHY_ADAPTER_INPROC",
		"BASHY_INVOKED_AS",
	} {
		_ = os.Unsetenv(k)
	}
}

// BashySelf returns the invoking bashy executable path passed by the adapter
// script (or an explicit BASHY_SELF override in tests).
func BashySelf() string {
	if v := strings.TrimSpace(os.Getenv("BASHY_SELF")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("BASHY_ADAPTER_SELF")); v != "" {
		return v
	}
	return startupBashySelf
}

// InvokedAs returns the verb spelling bashy received on argv[1], falling back
// to def when invoked directly outside bashy.
func InvokedAs(def string) string {
	if v := strings.TrimSpace(os.Getenv("BASHY_INVOKED_AS")); v != "" {
		return v
	}
	if startupInvokedAs != "" {
		return startupInvokedAs
	}
	return def
}

// DisableCompletion suppresses cobra's default completion subcommand.
func DisableCompletion(cmd *cobra.Command) {
	cmd.CompletionOptions.DisableDefaultCmd = true
}

// RunCobra configures cmd with bashy's separator normalizer, common options
// (--explain, --no-input, --json, --fields, --null), and S19 graph stream
// projections, then executes cmd against os.Args[1:].
func RunCobra(cmd *cobra.Command, errPrefix string, exitCode func(error) int) int {
	return RunCobraArgs(cmd, os.Args[1:], errPrefix, exitCode)
}

// RunCobraArgs is RunCobra with an explicit argument slice.
func RunCobraArgs(cmd *cobra.Command, args []string, errPrefix string, exitCode func(error) int) int {
	StripAdapterEnv()
	DisableCompletion(cmd)
	if certProfile() {
		cmd.SetArgs(args)
	} else {
		cmd.SetGlobalNormalizationFunc(normalizeFlagName)
		out, err := applyCommonOptions(cmd, args)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "bashy: %v\n", err)
			return 2
		}
		cmd.SetArgs(out)
	}
	err := cmd.Execute()
	if err == nil {
		return 0
	}
	if errPrefix != "" {
		fmt.Fprintln(os.Stderr, errPrefix, err)
	}
	if exitCode != nil {
		return exitCode(err)
	}
	return 1
}

func certProfile() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("VSC_PROFILE")), "cert")
}

func normalizeFlagName(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
}

func ownFlag(c *cobra.Command, name string) bool {
	return c.Flags().Lookup(name) != nil || c.PersistentFlags().Lookup(name) != nil || c.InheritedFlags().Lookup(name) != nil
}

func applyCommonOptions(root *cobra.Command, args []string) ([]string, error) {
	target, _, err := root.Find(args)
	if err != nil || target == nil {
		target = root
	}
	path := target.CommandPath()
	explain := false
	noInput := false
	nullFlag := ""
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		if !strings.HasPrefix(a, "--") {
			out = append(out, a)
			continue
		}
		name, val, hasVal := strings.Cut(a[2:], "=")
		norm := string(normalizeFlagName(nil, name))
		switch norm {
		case "explain", "no-input", "json", "fields", "null":
		default:
			out = append(out, a)
			continue
		}
		if ownFlag(target, norm) {
			out = append(out, a)
			continue
		}
		switch norm {
		case "explain":
			on := true
			if hasVal {
				b, perr := strconv.ParseBool(val)
				if perr != nil {
					return nil, fmt.Errorf("--explain: invalid value %q", val)
				}
				on = b
			}
			explain = on
		case "no-input":
			on := true
			if hasVal {
				b, perr := strconv.ParseBool(val)
				if perr != nil {
					return nil, fmt.Errorf("--no-input: invalid value %q", val)
				}
				on = b
			}
			noInput = on
		case "json":
			if hasVal {
				b, perr := strconv.ParseBool(val)
				if perr != nil {
					return nil, fmt.Errorf("--json: invalid value %q", val)
				}
				if !b {
					continue
				}
			}
			return nil, fmt.Errorf("--json unsupported for %s", path)
		case "fields":
			if hasVal && val == "" {
				continue
			}
			return nil, fmt.Errorf("--fields unsupported for %s (no native --fields or declared lens parser)", path)
		case "null":
			for _, n := range []string{"zero", "print0"} {
				if target.Flags().Lookup(n) != nil {
					nullFlag = n
					break
				}
			}
			if nullFlag == "" {
				return nil, fmt.Errorf("--null unsupported for %s (no native -z/--zero/--print0)", path)
			}
			out = append(out, "--"+nullFlag)
		}
	}
	if noInput {
		wrapRun(target, func(inner func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
			return func(c *cobra.Command, a []string) error {
				prev, had := os.LookupEnv("BASHY_NO_INPUT")
				_ = os.Setenv("BASHY_NO_INPUT", "1")
				defer func() {
					if had {
						_ = os.Setenv("BASHY_NO_INPUT", prev)
					} else {
						_ = os.Unsetenv("BASHY_NO_INPUT")
					}
				}()
				return inner(c, a)
			}
		})
	}
	installGraphStreams(target)
	if explain {
		explainOnly(target)
	}
	return out, nil
}

func wrapRun(c *cobra.Command, wrap func(func(*cobra.Command, []string) error) func(*cobra.Command, []string) error) {
	switch {
	case c.RunE != nil:
		c.RunE = wrap(c.RunE)
	case c.Run != nil:
		run := c.Run
		c.Run = nil
		c.RunE = wrap(func(c *cobra.Command, a []string) error { run(c, a); return nil })
	}
}

func explainOnly(c *cobra.Command) {
	for p := c; p != nil; p = p.Parent() {
		p.PersistentPreRun, p.PersistentPreRunE = nil, nil
		p.PersistentPostRun, p.PersistentPostRunE = nil, nil
	}
	c.PreRun, c.PreRunE, c.PostRun, c.PostRunE = nil, nil, nil, nil
	body := func(c *cobra.Command, args []string) error {
		w := c.OutOrStdout()
		fmt.Fprintf(w, "explain: %s (not executed)\n", c.CommandPath())
		if c.Short != "" {
			fmt.Fprintf(w, "  summary: %s\n", c.Short)
		}
		fmt.Fprintf(w, "  usage: %s\n", c.UseLine())
		if len(args) > 0 {
			fmt.Fprintf(w, "  operands: %s\n", strings.Join(args, " "))
		}
		c.Flags().Visit(func(f *pflag.Flag) {
			fmt.Fprintf(w, "  --%s=%s  %s\n", f.Name, f.Value.String(), f.Usage)
		})
		return nil
	}
	if c.Runnable() {
		c.Run = nil
		c.RunE = body
	}
}

type graphProjection func(raw []byte) (*stream.Graph, error)

var errNotProjectable = errors.New("graph stream: document is not projectable")

var graphProjections = map[string]graphProjection{
	"todo list": projectTodoList,
	"todo show": projectTodoShow,
}

func graphCommandPath(c *cobra.Command) string {
	fields := strings.Fields(c.CommandPath())
	if len(fields) == 0 {
		return ""
	}
	if fields[0] == "issue" {
		fields[0] = "todo"
	}
	return strings.Join(fields, " ")
}

func installGraphStreams(target *cobra.Command) {
	path := graphCommandPath(target)
	if p, ok := graphProjections[path]; ok && target.Flags().Lookup("json") != nil {
		wrapRun(target, func(inner func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
			return graphJSONRun(path, p, inner)
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
				noun, _, _ := strings.Cut(path, " ")
				return stream.Encode(real, g, noun, graphQuery(path, c, args))
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

type jsonObj = map[string]any

type graphBuilder struct {
	g     stream.Graph
	nodes map[string]bool
	edges map[[3]string]bool
}

func newGraphBuilder(source string) *graphBuilder {
	return &graphBuilder{g: stream.Graph{Source: source}, nodes: map[string]bool{}, edges: map[[3]string]bool{}}
}

func (b *graphBuilder) node(r, name, kind, typ string, attrs map[string]any) {
	if r == "" || b.nodes[r] {
		return
	}
	if name == "" {
		name = r
	}
	b.nodes[r] = true
	b.g.Nodes = append(b.g.Nodes, provider.Node{SchemaVersion: provider.NodeSchema, Ref: r, Name: name, Kind: kind, Type: typ, Attrs: attrs})
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

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
	b := newGraphBuilder(ref.Locator{Scheme: string(ref.Todo), Target: firstNonEmpty(str(res, "folder"), str(res, "scope"), "*")}.String())
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
