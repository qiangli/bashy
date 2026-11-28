package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/sharedfiles"
)

func main() {
	os.Exit(adapter.RunCobra(sharedfiles.NewCmd(), "bashy files:", nil))
}
