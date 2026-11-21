package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/yoke/cmds/hexdump"
)

func main() {
	toolrun.Main("hexdump")
}
