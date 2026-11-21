package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/yes"
)

func main() {
	toolrun.Main("yes")
}
