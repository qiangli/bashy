package main

import (
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/actrunner"
)

func main() {
	os.Exit(adapter.RunCobra(actrunner.NewCmd(), "", nil))
}
