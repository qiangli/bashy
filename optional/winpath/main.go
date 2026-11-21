package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/yoke/cmds/winpath"
)

func main() {
	toolrun.Main("wslpath")
}
