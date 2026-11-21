package main

import (
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/zot"
)

func main() {
	os.Exit(adapter.RunCobra(zot.NewZotCmd(), "", nil))
}
