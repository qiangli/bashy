package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/resources"
)

func main() {
	os.Exit(adapter.RunCobra(resources.NewCommand(), "bashy resource:", nil))
}
