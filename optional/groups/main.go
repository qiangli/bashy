package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/groups"
)

func main() {
	toolrun.Main("groups")
}
