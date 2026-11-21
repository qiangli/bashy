package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/install"
)

func main() {
	toolrun.Main("install")
}
