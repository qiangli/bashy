package main

import (
	"os"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/mise"
)

func main() {
	os.Exit(adapter.RunCobra(mise.NewMiseCmd(), "", nil))
}
