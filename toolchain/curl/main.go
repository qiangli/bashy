package main

import (
	"os"
	"runtime"
	"strings"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/curlbin"
)

func mapNullDevice(goos string, args []string) []string {
	if goos != "windows" {
		return args
	}
	out := make([]string, len(args))
	for i, a := range args {
		switch {
		case a == "/dev/null":
			a = "NUL"
		case strings.HasPrefix(a, "--") && strings.HasSuffix(a, "=/dev/null"):
			a = strings.TrimSuffix(a, "/dev/null") + "NUL"
		}
		out[i] = a
	}
	return out
}

func main() {
	os.Exit(adapter.RunCobraArgs(curlbin.NewCurlCmd(), mapNullDevice(runtime.GOOS, os.Args[1:]), "", nil))
}
