package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/shred"
)

func main() {
	toolrun.Main("shred")
}
