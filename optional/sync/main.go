package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/sync"
)

func main() {
	toolrun.Main("sync")
}
