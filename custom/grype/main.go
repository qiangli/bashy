package main

import (
	"errors"
	"os"
	"os/exec"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/grype"
)

func main() {
	os.Exit(adapter.RunCobra(grype.NewGrypeCmd(), "", func(err error) int {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return adapter.ChildExitStatus(err)
		}
		return 1
	}))
}
