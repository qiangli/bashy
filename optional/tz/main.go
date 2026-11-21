package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/yoke/cmds/tz"
)

func main() {
	toolrun.Main("tz")
}
