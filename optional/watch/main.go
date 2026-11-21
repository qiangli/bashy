package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/yoke/cmds/watch"
)

func main() {
	toolrun.Main("watch")
}
