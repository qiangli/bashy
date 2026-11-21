package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/dircolors"
)

func main() {
	toolrun.Main("dircolors")
}
