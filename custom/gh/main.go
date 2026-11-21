package main

import (
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/gh"
)

func main() {
	os.Exit(adapter.RunCobra(gh.NewGhCmd(), "", nil))
}
