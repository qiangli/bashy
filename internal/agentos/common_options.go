package agentos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// noInputEnv is an externally supplied switch; prompt helpers fail fast while
// it is set. --no-input itself never touches the environment: it raises
// noInputDepth for the duration of the command body only. Piped stdin payloads
// are untouched either way.
const noInputEnv = "BASHY_NO_INPUT"

var noInputDepth atomic.Int32

// normalizeFlagName makes "-" and "_" interchangeable in long flag names.
func normalizeFlagName(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
}

func noInputRequested() bool { return noInputDepth.Load() > 0 || os.Getenv(noInputEnv) != "" }

// OutputFlags is the one binder for the --json/--plain/--quiet triple on
// bashy-owned cobra commands (weave, dag and peers).
type OutputFlags struct{ cmd *cobra.Command }

// BindOutputFlags declares --json, --plain and --quiet on cmd where the
// command does not already own them, and returns the resolver.
func BindOutputFlags(cmd *cobra.Command) *OutputFlags {
	fs := cmd.Flags()
	for _, f := range []struct{ name, usage string }{
		{"json", "Machine-readable JSON output"},
		{"plain", "Plain-text output, no banners"},
		{"quiet", "Suppress non-essential output"},
	} {
		if !ownFlag(cmd, f.name) {
			fs.Bool(f.name, false, f.usage)
		}
	}
	return &OutputFlags{cmd: cmd}
}

// Mode resolves the output mode the way weavecli does.
func (o *OutputFlags) Mode() weavecli.OutputMode {
	boolFlag := func(name string) (set, val bool) {
		f := o.cmd.Flags().Lookup(name)
		if f == nil {
			return false, false
		}
		return f.Changed, f.Value.String() == "true"
	}
	jsonSet, jsonVal := boolFlag("json")
	_, plain := boolFlag("plain")
	_, quiet := boolFlag("quiet")
	return weavecli.ResolveOutputModeEx(jsonSet, jsonVal, plain, quiet)
}

// NormalizeOptionKey maps a YAML or environment key (dry_run, DRY_RUN,
// dryrun, dry-run) to its canonical flag name (dry-run).
func NormalizeOptionKey(key string) string {
	k := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(key)), "_", "-")
	if k == "dryrun" {
		return "dry-run"
	}
	return k
}

// OptionKey is the official YAML/env spelling of a flag name (dry-run -> dry_run).
func OptionKey(flag string) string {
	return strings.ReplaceAll(NormalizeOptionKey(flag), "-", "_")
}

// setCobraArgs is the single chokepoint for bashy-owned cobra verbs: it
// installs the separator normalizer and the engine common options, then sets
// argv. Both are off in the cert profile.
func setCobraArgs(cmd *cobra.Command, args []string) {
	if certProfile() {
		cmd.SetArgs(args)
		return
	}
	cmd.SetGlobalNormalizationFunc(normalizeFlagName)
	out, err := applyCommonOptions(cmd, args)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "bashy: %v\n", err)
		dispatchExit(2)
		return
	}
	cmd.SetArgs(out)
}

type commonOpts struct {
	explain, json bool
	fields        string
}

func ownFlag(c *cobra.Command, name string) bool {
	return c.Flags().Lookup(name) != nil || c.PersistentFlags().Lookup(name) != nil || c.InheritedFlags().Lookup(name) != nil
}

// applyCommonOptions strips engine-handled common options from args (a
// command's own flag of the same name always wins) and arranges their
// behaviour on the resolved command. A non-nil error is a native exit 2.
func applyCommonOptions(root *cobra.Command, args []string) ([]string, error) {
	target, _, err := root.Find(args)
	if err != nil || target == nil {
		target = root
	}
	path := target.CommandPath()
	var o commonOpts
	hasFields, noInput := false, false
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
			o.explain = true
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
			return nil, fmt.Errorf("--json unsupported for %s", path)
		case "fields":
			if !hasVal {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("--fields requires a value")
				}
				i++
				val = args[i]
			}
			if strings.TrimSpace(strings.ReplaceAll(val, ",", "")) == "" {
				return nil, fmt.Errorf("--fields requires at least one field name")
			}
			o.fields, hasFields = val, true
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
	if hasFields {
		if !ownFlag(target, "json") {
			return nil, fmt.Errorf("--fields unsupported for %s (no native --json)", path)
		}
		o.json = true
		out = withJSONFlag(out)
		wrapRun(target, func(inner func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
			return func(c *cobra.Command, a []string) error {
				real := c.OutOrStdout()
				var buf bytes.Buffer
				c.SetOut(&buf)
				runErr := inner(c, a)
				c.SetOut(real)
				if err := writeProjected(real, buf.Bytes(), strings.Split(o.fields, ",")); err != nil {
					return err
				}
				return runErr
			}
		})
	}
	if noInput {
		wrapRun(target, func(inner func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
			return func(c *cobra.Command, a []string) error {
				noInputDepth.Add(1)
				defer noInputDepth.Add(-1)
				return inner(c, a)
			}
		})
	}
	if o.explain {
		explainOnly(target)
	}
	return out, nil
}

// withJSONFlag makes --fields imply --json unless the caller already gave it.
func withJSONFlag(args []string) []string {
	end := len(args)
	for i, a := range args {
		if a == "--" {
			end = i
			break
		}
		if n, _, _ := strings.Cut(strings.TrimPrefix(a, "--"), "="); strings.HasPrefix(a, "--") && string(normalizeFlagName(nil, n)) == "json" {
			return args
		}
	}
	res := append([]string{}, args[:end]...)
	res = append(res, "--json")
	return append(res, args[end:]...)
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

// explainOnly replaces the command body (and every hook above it) with a
// description of the parsed argv; flag and operand validation still runs.
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

func writeProjected(w io.Writer, raw []byte, fields []string) error {
	trimmed := bytes.TrimSpace(raw)
	var v any
	if json.Unmarshal(trimmed, &v) == nil {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(projectJSON(v, fields))
	}
	var lines []any
	for _, ln := range bytes.Split(trimmed, []byte("\n")) {
		if len(bytes.TrimSpace(ln)) == 0 {
			continue
		}
		var rec any
		if json.Unmarshal(ln, &rec) != nil {
			_, err := w.Write(raw)
			return err
		}
		lines = append(lines, projectJSON(rec, fields))
	}
	enc := json.NewEncoder(w)
	for _, l := range lines {
		if err := enc.Encode(l); err != nil {
			return err
		}
	}
	return nil
}

func projectJSON(v any, fields []string) any {
	switch t := v.(type) {
	case []any:
		res := make([]any, len(t))
		for i, e := range t {
			res[i] = projectJSON(e, fields)
		}
		return res
	case map[string]any:
		if items, ok := t["items"].([]any); ok {
			t["items"] = projectJSON(items, fields)
			return t
		}
		res := map[string]any{}
		for _, f := range fields {
			if f = strings.TrimSpace(f); f != "" {
				if val, ok := t[f]; ok {
					res[f] = val
				}
			}
		}
		return res
	}
	return v
}
