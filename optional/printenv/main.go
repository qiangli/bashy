package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/printenv"
)

func main() {
	toolrun.Main("printenv")
}
