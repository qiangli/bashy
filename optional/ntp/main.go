package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/yoke/cmds/ntp"
)

func main() {
	toolrun.Main("ntp")
}
