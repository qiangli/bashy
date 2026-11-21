package main

import (
	"os"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/node"
)

func main() {
	os.Exit(adapter.RunCobra(node.NewNodeCmd(), "", nil))
}
