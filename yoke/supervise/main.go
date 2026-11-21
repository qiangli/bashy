package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/supervise"
)

func main() {
	os.Exit(adapter.RunCobra(supervise.NewSuperviseCmd(), "bashy supervise:", nil))
}
