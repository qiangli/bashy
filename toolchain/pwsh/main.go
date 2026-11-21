package main

import (
	"os"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/pwsh"
)

func main() {
	os.Exit(adapter.RunCobra(pwsh.NewCmd(), "", nil))
}
