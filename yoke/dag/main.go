package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/dag"
)

func main() {
	cmd := dag.NewDagCmd()
	dag.AddCapacityCommands(cmd, dag.CapacityServices{})
	os.Exit(adapter.RunCobra(cmd, "bashy dag:", func(err error) int {
		var dagErr *dag.Error
		if err != nil && !errors.As(err, &dagErr) {
			fmt.Fprintf(os.Stderr, "bashy dag: %v\n", err)
		}
		return dag.ExitCodeOf(err)
	}))
}
