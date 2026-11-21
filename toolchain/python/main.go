package main

import (
	"os"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/python"
)

func main() {
	os.Exit(adapter.RunCobra(python.NewPythonCmd(), "", nil))
}
