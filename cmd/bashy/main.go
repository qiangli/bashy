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

	"github.com/qiangli/coreutils/pkg/telemetry"

	"github.com/qiangli/bashy/internal/agentos"
	"github.com/qiangli/bashy/internal/cli"
)

func init() {
	cli.AgentOSDispatch = agentos.Dispatch
	cli.AgentOSWireExec = agentos.WireExec
	cli.AgentOSPreamble = agentos.Preamble
	cli.AgentOSUsage = agentos.Usage
	// Keep the fork's nohup/setsid builtins: the in-process matrix shell needs
	// `nohup foo &` to outlive a closed SSH session, which an external nohup
	// over a goroutine job can't provide. (The pure `bash` drop-in suppresses
	// them for strict bash 5.3 fidelity.)
	cli.SuppressedForkBuiltins = nil
}

func main() {
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
