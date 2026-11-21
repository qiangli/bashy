package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
)

func main() {
	os.Exit(adapter.RunCobra(newKBCmd(), "bashy kb:", kbExitCode))
}
