package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/bashsharp/bashsharp/cli"
	"github.com/qiangli/yoke/external/meshagent"
)

func main() {
	meshagent.HostVersion = cli.BashyVersion
	if err := meshagent.Exec(context.Background(), os.Args[1:]...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}
