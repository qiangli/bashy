package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/sha256sum"
)

func main() {
	toolrun.Main("sha256sum")
}
