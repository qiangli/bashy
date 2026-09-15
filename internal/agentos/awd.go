// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// `bashy awd DIR -- CMD [ARGS…]` runs ONE command in another directory and
// comes back — the front-door form of the `awd` shell builtin (the in-process
// answer to `cd DIR && CMD` that never strands the shell in DIR). It exists so
// that no verb has to grow a `-C`/`-D` directory flag of its own: `bashy dag`,
// `bashy git`, a registered command, an applet — every one of them is "run it
// over there" through this single mechanism.
//
//	bashy awd ~/src/app -- bashy dag -f ~/pipelines/python.dag.md test
//	bashy awd /tmp -- pwd
//
// The builtin already works inside any bashy script or `-c` program; this file
// only routes the bare `bashy awd …` invocation to it (the same self-reexec
// shape as `bashy full`), so cwd, exit status, stdio and BASHY_* env behave
// exactly as they do for the builtin.
package agentos

import (
	"fmt"
	"os"
	"os/exec"
)

// dispatchAwd implements `bashy awd`. It runs before shell flag parsing, so it
// uses the process stdio/cwd/env directly and returns CMD's own exit status.
func dispatchAwd(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(os.Stderr, "usage: bashy awd DIR -- command [args...]")
		fmt.Fprintln(os.Stderr, "  run one command in DIR and return; the cwd of the caller is untouched")
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	// `awd "$@"` hands the operands to the builtin untouched, `--` included,
	// so the builtin's own parsing (and its own diagnostics) apply.
	cmdArgs := append([]string{"-c", `awd "$@"`, "bashy awd"}, args...)
	cmd := exec.Command(bashySelfPath(), cmdArgs...)
	cmd.Env = os.Environ()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "bashy awd:", err)
		return 127
	}
	_ = cmd.Wait()
	status, _ := procStatus(cmd.ProcessState)
	return status
}
