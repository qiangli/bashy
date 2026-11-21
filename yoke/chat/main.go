package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/chat"
)

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

func main() {
	cmd := chat.NewChatCmd()
	cmd.SilenceErrors = true
	os.Exit(adapter.RunCobra(cmd, "bashy chat:", func(err error) int {
		if err == nil {
			return 0
		}
		if chatDryRunUnroutable(os.Args[1:], err) {
			fmt.Fprintf(os.Stdout, "dry-run: plan is valid but not routable on this host: %s\n", err)
			return 0
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		if code := chat.ExitCode(err); code > 1 {
			return code
		}
		return 1
	}))
}
