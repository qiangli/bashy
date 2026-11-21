package main

import (
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/kopia"
)

func main() {
	os.Exit(adapter.RunCobra(kopia.NewKopiaCmd(), "", nil))
}
