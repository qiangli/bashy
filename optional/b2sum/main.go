package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/b2sum"
)

func main() {
	toolrun.Main("b2sum")
}
