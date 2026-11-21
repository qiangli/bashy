package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/seq"
)

func main() {
	toolrun.Main("seq")
}
