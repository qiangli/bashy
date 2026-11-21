package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/readlink"
)

func main() {
	toolrun.Main("readlink")
}
