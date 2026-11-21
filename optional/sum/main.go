package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/sum"
)

func main() {
	toolrun.Main("sum")
}
