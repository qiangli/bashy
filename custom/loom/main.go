package main

import (
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/loom"
)

func main() {
	os.Exit(adapter.RunCobra(loom.NewLoomCmd(), "", nil))
}
