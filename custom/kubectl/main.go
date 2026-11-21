package main

import (
	"os"
	"runtime"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/kubectl"
)

func main() {
	if runtime.GOOS == "windows" {
		adapter.AdaptKubeEnvironment()
	}
	args := adapter.NativeArgsOnWindows(os.Args[1:])
	os.Exit(adapter.RunCobraArgs(kubectl.NewKubectlCmd(), args, "", nil))
}
