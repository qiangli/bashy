package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/chcon"
)

func main() {
	toolrun.Main("chcon")
}
