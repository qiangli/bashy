package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/uptime"
)

func main() {
	toolrun.Main("uptime")
}
