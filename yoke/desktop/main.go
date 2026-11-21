package main

import (
	"fmt"
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/pkg/accounts/builtin"
	"github.com/qiangli/yoke/pkg/fleet"
)

func main() {
	cmd := fleet.NewDesktopCmd()
	code := adapter.RunCobraArgs(cmd, os.Args[1:], "", func(err error) int {
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			fmt.Fprintln(os.Stdout, fleet.DesktopErrorJSON(err))
		}
		fmt.Fprintln(os.Stderr, "bashy desktop:", err)
		return fleet.DesktopExitCode(err)
	})
	os.Exit(code)
}
