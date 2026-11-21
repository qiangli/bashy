package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/gate"
)

func main() {
	os.Exit(adapter.RunCobra(gate.NewGateCmd(), "bashy gate:", nil))
}
