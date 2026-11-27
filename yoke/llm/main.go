package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/broker"
)

func main() {
	// `llm up` starts the door by re-running this adapter as `llm serve`; the
	// in-process bashy route sets the same seam (internal/agentos/door.go).
	// Without it `bashy llm up` always failed "no self command".
	broker.SelfArgv = func() []string {
		exe, err := os.Executable()
		if err != nil {
			exe = os.Args[0]
		}
		return []string{exe, "serve"}
	}
	os.Exit(adapter.RunCobra(broker.NewCmd(), "bashy llm:", nil))
}
