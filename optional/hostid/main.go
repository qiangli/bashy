package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/hostid"
)

func main() {
	toolrun.Main("hostid")
}
