package main

import (
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/act"
)

func main() {
	os.Exit(adapter.RunCobra(act.NewActCmd(), "", nil))
}
