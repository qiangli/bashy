package main

import (
	"os"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/rust"
)

func main() {
	os.Exit(adapter.RunCobra(rust.NewRustcCmd(), "", nil))
}
