package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/capability"
)

func main() {
	os.Exit(adapter.RunCobra(capability.NewCapabilityCmd(), "bashy capability:", nil))
}
