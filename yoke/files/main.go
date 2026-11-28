package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	// Sharing a folder with an audience registers its offer app: pkg/offer
	// wires the sharedfiles hooks in its init (Sprint 739).
	_ "github.com/qiangli/yoke/pkg/offer"
	"github.com/qiangli/yoke/pkg/sharedfiles"
)

func main() {
	os.Exit(adapter.RunCobra(sharedfiles.NewCmd(), "bashy files:", nil))
}
