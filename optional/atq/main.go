package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/yoke/cmds/atq"
)

func main() {
	toolrun.Main("atq")
}
