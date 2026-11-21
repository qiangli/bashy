package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/adapt"
)

func main() {
	cmd := adapt.NewCmd(adapt.PrepareJSONLens)
	adapter.DisableCompletion(cmd)
	cmd.SetArgs(os.Args[1:])
	os.Exit(adapt.ExitCode(cmd.Execute()))
}
