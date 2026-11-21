package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/external/sphere"
)

func main() {
	os.Exit(adapter.RunCobra(sphere.NewSphereCmd(), "", nil))
}
