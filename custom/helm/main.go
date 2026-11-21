package main

import (
	"os"
	"runtime"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/helm"
)

func main() {
	if runtime.GOOS == "windows" {
		adapter.AdaptKubeEnvironment()
	}
	args := adapter.NativeArgsOnWindows(os.Args[1:])
	os.Exit(adapter.RunCobraArgs(helm.NewHelmCmd(), args, "", nil))
}
