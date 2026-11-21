package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/stats"
)

func main() {
	os.Exit(adapter.RunCobra(stats.NewCmd(), "bashy stats:", nil))
}
