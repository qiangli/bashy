package main

import (
	"os"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/clang"
)

func main() {
	os.Exit(adapter.RunCobra(clang.NewClangCmd(), "", nil))
}
