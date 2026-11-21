package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/external/tessaro"
)

func main() {
	os.Exit(adapter.RunCobra(tessaro.NewTessaroCmd(), "", nil))
}
