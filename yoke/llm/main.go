package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/broker"
)

func main() {
	os.Exit(adapter.RunCobra(broker.NewCmd(), "bashy llm:", nil))
}
