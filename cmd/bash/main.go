// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Command bash is the pure GNU Bash 5.3 drop-in: the sh engine and CLI only,
// with no AgentOS surface. Its import graph deliberately does NOT include
// coreutils or any external-tool wiring, so it stays lean and is exactly what
// the bash 5.3 compliance harness measures. The `bashy` system shell lives in
// the sibling cmd/bashy and is built independently.
package main

import "github.com/qiangli/bashy/internal/cli"

func main() { cli.Main() }
