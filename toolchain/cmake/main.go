package main

import (
	"os"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/cmake"
)

func main() {
	os.Exit(adapter.RunCobra(cmake.NewCmakeCmd(), "", nil))
}
