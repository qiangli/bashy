package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/factor"
)

func main() {
	toolrun.Main("factor")
}
