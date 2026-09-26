// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Command bashy is the AgentOS system shell: the same shell core as `bash`,
// plus the coreutils pure-Go userland, the `yc` code-intel verbs, and the
// front-door subcommands (`bashy weave …`, `bashy podman …`). It is the
// self-contained bootstrapper for a whole unix-like userland (bash + coreutils
// + pkg + external tools). Built independently of cmd/bash; the coreutils
// import lives only here (via internal/agentos), so the pure `bash` drop-in
// never carries it.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/qiangli/yoke/pkg/telemetry"

	"github.com/qiangli/coreutils/tool"

	"github.com/qiangli/bashy/internal/agentos"
	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/ycode/pkg/ycodecli"
	"mvdan.cc/sh/v3/interp/ownedexec"

	// Embedded CA roots, used only when the system has none: the offline
	// image is FROM scratch (no /etc/ssl), and bashy provisions its own
	// toolchains (uv/CPython, Go, …) over HTTPS from inside it.
	_ "golang.org/x/crypto/x509roots/fallback"
)

func init() {
	// `bashy ycode`: the YAML agent engine, wired here rather than imported by
	// internal/agentos — ycode imports bashy's pkg/harnessrunner, which imports
	// agentos, so only package main can close the loop.
	agentos.YcodeMain = ycodecli.Main
	// The engine's terminal frontend is bashy's default agent TUI: the
	// interactive shell with the agent attached (Sprint #301 T4).
	ycodecli.TerminalUI = func(ctx context.Context, s ycodecli.TerminalSession) error {
		return cli.RunAgentTerminal(ctx, s.Agent, s.Config, s.Session)
	}
	cli.AgentOSOwnedCommand = func(name string) bool { return tool.Lookup(name) != nil }
	cli.AgentOSOwnedNames = tool.Names
	cli.AgentOSDispatch = agentos.Dispatch
	cli.AgentOSWireExec = agentos.WireExec
	cli.AgentOSPreamble = agentos.Preamble
	cli.AgentOSUsage = agentos.Usage
	cli.AgentOSCommandLineNoExec = agentos.PosixDryRunNoExec
	cli.AgentOSStrictPosixParse = agentos.PosixDryRunNoExec
	cli.AgentOSBashPPDefault = true
	cli.VersionProduct = "bashy"
	cli.VersionCompatibility = "GNU Bash 5.3 compatible"
	// Keep the fork's nohup/setsid builtins: the in-process matrix shell needs
	// `nohup foo &` to outlive a closed SSH session, which an external nohup
	// over a goroutine job can't provide. (The pure `bash` drop-in suppresses
	// them for strict bash 5.3 fidelity.)
	cli.SuppressedForkBuiltins = nil
}

func main() {
	adoptingOwnedFrame := len(os.Args) == 2 && os.Args[1] == ownedexec.Sentinel
	if err := ownedexec.Adopt(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(126)
	}
	if adoptingOwnedFrame {
		cli.AdoptGoSourceProcessEnvironment()
	}
	// Job-carrier helper mode (a re-exec of this binary standing in for one
	// background job) must not initialize telemetry or any AgentOS surface:
	// intercept it before everything. cli.Main also intercepts, but by then
	// telemetry.Init below would already have run. Never returns in helper mode.
	cli.MaybeRunJobCarrierHelper()

	// The OTel plane. A no-op unless OTEL_EXPORTER_OTLP_ENDPOINT is set — no exporter,
	// no batcher, no goroutine, no cost — so an ordinary interactive shell pays nothing.
	//
	// It lives in cmd/bashy and NOT in the shared internal/cli, because cmd/bash is the
	// pure Bash 5.3 drop-in and must stay lean and behaviourally exact. A drop-in that
	// dials a collector is not a drop-in.
	shutdown := telemetry.Init(context.Background())

	// NOT `defer`. A shell exits with os.Exit, and OS.EXIT DOES NOT RUN DEFERS.
	//
	// The first version of this used defer. It compiled, five unit tests passed against
	// an in-memory span recorder, and a real collector received ZERO BYTES — the batch
	// processor was never flushed, so every span died in memory at exit. A test that
	// mocks the emitter proves the emitter was called; it does not prove the data
	// arrived.
	cli.AgentOSShutdown = func() { _ = shutdown(context.Background()) }

	cli.Main()
}
