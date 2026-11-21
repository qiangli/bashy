package main

import (
	"os"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/gotoolchain"
)

func main() {
	adapter.ApplyAllProxyFallback()
	os.Exit(adapter.RunCobra(gotoolchain.NewGoCmd(), "", nil))
}
