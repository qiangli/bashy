package agentos

import (
	"fmt"
	"io"
	"strings"

	"github.com/qiangli/yoke/pkg/chat"
)

// runChatCmd runs `bashy chat` and returns its exit code. A --dry-run that
// parsed fine but selects a tool/band no installed agent can serve is a plan,
// not an error: it reports the unroutable reason and exits 0. Usage errors
// (bad flags, out-of-range band) and non-dry-run failures stay non-zero.
func runChatCmd(args []string, stdout, stderr io.Writer) int {
	cmd := chat.NewChatCmd()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SilenceErrors = true
	err := cmd.Execute()
	if err == nil {
		return 0
	}
	if chatDryRunUnroutable(args, err) {
		fmt.Fprintf(stdout, "dry-run: plan is valid but not routable on this host: %s\n", err)
		return 0
	}
	fmt.Fprintln(stderr, "Error:", err)
	return 1
}

func chatDryRunUnroutable(args []string, err error) bool {
	dry := false
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--dry-run" || a == "--dry-run=true" {
			dry = true
		}
	}
	return dry && strings.Contains(err.Error(), "chat: no operable agent matches")
}
