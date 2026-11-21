package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var noInputDepth atomic.Int32

// normalizeFlagName makes "-" and "_" interchangeable in long flag names.
func normalizeFlagName(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
}

type commonOpts struct {
	explain bool
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
			o.explain = on
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
				noInputDepth.Add(1)
				defer noInputDepth.Add(-1)
				return inner(c, a)
			}
		})
	}
	installGraphStreams(target)
	if o.explain {
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
