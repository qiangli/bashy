package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/yoke/cmds/gzip"
)

func main() {
	toolrun.Main("gzip")
}
