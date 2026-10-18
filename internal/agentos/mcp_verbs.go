// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/qiangli/coreutils/tool"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/atlas"
)

// Tests replace this seam so front-door dispatch never re-runs the test binary.
var mcpVerbExec = func(ctx context.Context, argv []string, stdin, dir string) (stdout, stderr string, exit int) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir, cmd.Env, cmd.Stdin = dir, os.Environ(), strings.NewReader(stdin)
	var out, errout bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errout
	if err := cmd.Run(); err != nil {
		if cmd.ProcessState == nil {
			return out.String(), errout.String() + err.Error(), 126
		}
		return out.String(), errout.String(), cmd.ProcessState.ExitCode()
	}
	return out.String(), errout.String(), 0
}

var mcpMissingVerbAtlas sync.Map

func mcpVerbCommands() []yokemcp.RegisteredCommand {
	_, _, verbs := commandsCatalog()
	hidden := hiddenVerbsCatalog()
	ring := registeredNames()
	ring = append(ring, registeredHiddenNames()...)
	var out []yokemcp.RegisteredCommand
	for _, name := range slices.Compact(verbs) {
		if slices.Contains(hidden, name) || slices.Contains(ring, name) || tool.Lookup(name) != nil {
			continue
		}
		entry, ok := atlas.Lookup(name)
		if !ok {
			if _, logged := mcpMissingVerbAtlas.LoadOrStore(name, true); !logged {
				slog.Debug("MCP verb skipped: no atlas entry", "name", name)
			}
			continue
		}
		if entry.AliasOf != "" || !slices.Contains(entry.OS, runtime.GOOS) {
			continue
		}
		out = append(out, mcpVerbCommand(name, entry))
	}
	return out
}

// mcpVerbCommand is the atlas adapter shared by server discovery and help.
func mcpVerbCommand(name string, entry atlas.Entry) yokemcp.RegisteredCommand {
	// Derived flag schemas are descriptive, not an ArgSchema binding: the
	// latter injects defaults and cannot represent arbitrary operands or all
	// pflag types. Keep the existing argv transport and classic behavior intact.
	return yokemcp.RegisteredCommand{
		Usage: cobraSchemaSummary(name, cobraSchemas(name)),
		Name:  name, Synopsis: synopsisOf(name), Effects: slices.Clone(entry.Effects), OS: slices.Clone(entry.OS), Static: true,
		Run: func(ctx context.Context, args []string, stdin, dir string) (string, string, int) {
			return mcpVerbExec(ctx, append([]string{bashySelfPath(), name}, args...), stdin, dir)
		},
	}
}

// cobraSchemaSummary adds at most 2 KiB to each discovery description. Full
// schemas are disclosed only by explicit MCP help, never by tools/list.
const mcpVerbSummaryLimit = 2048

func cobraSchemaSummary(name string, schemas map[string]tool.ArgSchema) string {
	if schemas == nil {
		return ""
	}
	footer := "; Full per-path flag schemas: bashy help " + name + " --format=mcp (descriptive; invoke via args)."
	const truncated = "; ... (summary truncated)"
	var b strings.Builder
	appendPart := func(part string) bool {
		if b.Len()+len(part)+len(truncated)+len(footer) > mcpVerbSummaryLimit {
			b.WriteString(truncated)
			return false
		}
		b.WriteString(part)
		return true
	}
	for _, flag := range schemas[name].Flags {
		if !appendPart(fmt.Sprintf("; --%s (%s)", flag.Name, flag.Type)) {
			return b.String() + footer
		}
	}
	paths := make([]string, 0, len(schemas)-1)
	for path := range schemas {
		if path != name {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	for _, path := range paths {
		if !appendPart("; Command: " + path) {
			break
		}
	}
	return b.String() + footer
}
