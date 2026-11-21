package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/yoke/cmds/jq"
)

func main() {
	toolrun.Main("jq")
}
