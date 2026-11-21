package main

import (
	"os"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/zigcc"
)

func main() {
	os.Exit(adapter.RunCobra(zigcc.NewZigCmd(), "", nil))
}
