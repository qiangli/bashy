// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/webconsole"
)

// dispatchMeta answers `bashy <verb> meta [--json]` for any verb that declares
// an atlas.WebSurface, printing the SAME dhnt-app-meta-v1 contract a
// third-party app speaks through `<bin> meta --json`.
//
// One schema, three surfaces (`<bin> meta`, `bashy <verb> meta`, GET /meta), so
// bashy is the reference implementation of the contract it asks others to
// speak: an app author can read a working example by running a tool already on
// their machine.
//
// SCOPE IS DELIBERATELY NARROW. It claims the word only for verbs that declare
// a surface — four today — and only when `meta` is the whole remaining
// invocation. `bashy grep meta notes.txt` must keep meaning grep, and
// `bashy mb send meta "..."` must keep meaning send; a blanket intercept would
// quietly break both. This is why a verb WITHOUT a surface is not answered here
// at all rather than being told it has none.
func dispatchMeta(args []string) {
	if len(args) < 3 || args[2] != "meta" {
		return
	}
	asJSON := false
	switch len(args) {
	case 3:
	case 4:
		if args[3] != "--json" && args[3] != "-json" {
			return
		}
		asJSON = true
	default:
		return
	}

	verb := args[1]
	w, ok := atlas.WebSurfaces()[verb]
	if !ok {
		return // not ours to answer — let the verb have its argument back
	}
	m := webconsole.FromSurface(verb, w)

	if asJSON {
		if err := webconsole.WriteMeta(os.Stdout, m); err != nil {
			fmt.Fprintln(os.Stderr, "bashy "+verb+" meta:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(tw, "%s\t%s\n", k, v)
		}
	}
	row("schema", m.SchemaVersion)
	row("name", m.Name)
	row("label", m.Label)
	row("mount", m.Mount)
	row("mode", m.Mode)
	if m.Port != 0 {
		row("port", fmt.Sprint(m.Port))
	}
	row("auth", m.Auth)
	row("icon", m.Icon)
	row("tip", m.Tip)
	if len(m.Start) > 0 {
		row("start", "bashy "+strings.Join(m.Start, " "))
	}
	_ = tw.Flush()
	os.Exit(0)
}
