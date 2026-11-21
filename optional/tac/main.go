package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/tac"
)

func main() {
	toolrun.Main("tac")
}
