package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/realpath"
)

func main() {
	toolrun.Main("realpath")
}
