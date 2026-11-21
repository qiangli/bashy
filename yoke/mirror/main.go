package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/mirror"
)

func main() {
	os.Exit(adapter.RunCobra(mirror.NewMirrorCmd(), "", nil))
}
