package agentos

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/coreutils/tool"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/fleet"
	"mvdan.cc/sh/v3/syntax"
)

// Use the same request-local interpreter and exec ladder as the shell front
// door. In particular, never execute bashySelfPath to evaluate script bytes.
func mcpRunScript(ctx context.Context, script, stdin, dir string) (string, string, int, error) {
	var stdout, stderr bytes.Buffer
	exit := cli.RunSessionCommandWithConfig(ctx, cli.SessionIO{
		Command: script, Dir: dir, Env: os.Environ(), Stdin: strings.NewReader(stdin),
		Stdout: &stdout, Stderr: &stderr,
	}, cli.SessionConfig{WireExec: WireSessionExec(false), Preamble: func() string { return PreambleFor(false) }, ExecProcessGroups: true})
	return stdout.String(), stderr.String(), exit, nil
}

func mcpRegisteredSchema(args *fleet.CommandSchema) *tool.ArgSchema {
	if args == nil {
		return nil
	}
	schema := &tool.ArgSchema{}
	for _, p := range args.Positionals {
		schema.Positionals = append(schema.Positionals, tool.ArgParameter{
			Name: p.Name, Type: p.Type, Required: p.Required, Default: p.Default, Enum: slices.Clone(p.Enum),
		})
	}
	for _, f := range args.Flags {
		schema.Flags = append(schema.Flags, tool.ArgFlag{
			Name: f.Name, Shorthand: f.Shorthand, Type: f.Type, Required: f.Required, Default: f.Default, Enum: slices.Clone(f.Enum),
		})
	}
	return schema
}

func mcpRegisteredCommands() []yokemcp.RegisteredCommand {
	// The shell's directory fingerprint detects adds/removals. Force a reload
	// here as well so an in-place YAML edit updates the schema and dispatch rung.
	registeredMu.Lock()
	registeredIdx = nil
	registeredMu.Unlock()
	var out []yokemcp.RegisteredCommand
	for _, rec := range registeredCommands() {
		name := rec.Name
		out = append(out, yokemcp.RegisteredCommand{
			Name: name, Synopsis: rec.Synopsis, Usage: rec.Long,
			Effects: slices.Clone(rec.Effects), OS: slices.Clone(rec.OS), Schema: mcpRegisteredSchema(rec.Args),
			Run: func(ctx context.Context, argv []string, stdin, dir string) (string, string, int) {
				// Quote every word; shell metacharacters in structured arguments are data.
				// Entering the normal interpreter reuses registeredResolver's binder and
				// registeredHandler's download/exec/script/env/cwd dispatch unchanged.
				words := make([]string, 0, len(argv)+1)
				for _, word := range append([]string{name}, argv...) {
					quoted, err := syntax.Quote(word, syntax.LangBash)
					if err != nil {
						return "", err.Error(), 2
					}
					words = append(words, quoted)
				}
				stdout, stderr, exit, _ := mcpRunScript(ctx, strings.Join(words, " "), stdin, dir)
				return stdout, stderr, exit
			},
		})
	}
	return out
}

// os.ReadDir sorts names. Include each file's mtime, since editing a record
// in place need not change the containing directory's mtime.
func mcpRingFingerprint() string {
	var out strings.Builder
	for _, dir := range registeredCatalog().CommandDirs() {
		fmt.Fprintf(&out, "%q:", dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Fprintf(&out, "%v;", err)
			continue
		}
		for _, entry := range entries {
			info, err := os.Stat(filepath.Join(dir, entry.Name()))
			if err != nil {
				fmt.Fprintf(&out, "%q:%v;", entry.Name(), err)
				continue
			}
			fmt.Fprintf(&out, "%q:%d:%d;", entry.Name(), info.ModTime().UnixNano(), info.Size())
		}
	}
	return out.String()
}

func watchMCPRegistered(ctx context.Context, srv *mcpsdk.Server, opts yokemcp.Options, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	// Refresh on the first tick too, closing the build-to-watch startup gap.
	previous := ""
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				current := mcpRingFingerprint()
				if current == previous {
					continue
				}
				if err := yokemcp.NotifyToolsChanged(srv, opts); err != nil {
					fmt.Fprintln(os.Stderr, "bashy mcp: refresh registered commands:", err)
					continue
				}
				previous = current
			}
		}
	}()
	return done
}
