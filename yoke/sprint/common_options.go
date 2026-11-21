package main

import (
	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/spf13/cobra"
)

// OutputFlags is the one binder for the --json/--plain/--quiet triple on
// bashy-owned cobra commands (weave, dag and peers).
type OutputFlags struct{ cmd *cobra.Command }

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
