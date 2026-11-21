package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/fleet"
)

func main() {
	os.Exit(adapter.RunCobra(fleet.NewAdaptersCmd(), "bashy adapter:", nil))
}
