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
	"github.com/qiangli/bashy/internal/agentos"
	"github.com/qiangli/bashy/internal/cli"
)

func init() {
	cli.AgentOSDispatch = agentos.Dispatch
	cli.AgentOSWireExec = agentos.WireExec
}

func main() { cli.Main() }
