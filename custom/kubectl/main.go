package main

import (
	"os"
	"runtime"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/kubectl"
)

func prepareKubeEnvironment(goos string) {
	if goos == "windows" {
		adapter.AdaptKubeEnvironment()
	}
}

func main() {
	prepareKubeEnvironment(runtime.GOOS)
	args := adapter.NativeArgsOnWindows(os.Args[1:])
	os.Exit(adapter.RunCobraArgs(kubectl.NewKubectlCmd(), args, "", nil))
}
