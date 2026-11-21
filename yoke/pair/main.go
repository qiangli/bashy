package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/pair"
)

func main() {
	os.Exit(adapter.RunCobra(pair.NewPairCmd(), "bashy pair:", nil))
}
