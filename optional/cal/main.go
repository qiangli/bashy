package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/yoke/cmds/cal"
)

func main() {
	toolrun.Main("cal")
}
