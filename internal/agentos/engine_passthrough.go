package agentos

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/qiangli/yoke/pkg/binmgr"
)

// execEnginePassthrough runs a managed engine or the verified embedded runner
// and forwards termination so its child cannot outlive bashy.
func execEnginePassthrough(bin string, args []string) int {
	cmd := binmgr.Command(context.Background(), bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if podmanStartsMachine(args) {
		cmd.Dir = engineDaemonDir()
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "bashy: %s: %v\n", bin, err)
		return 1
	}
	stop := forwardSignals(cmd.Process)
	err := cmd.Wait()
	stop()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "bashy: %s: %v\n", bin, err)
		return 1
	}
	return 0
}
