package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/shuf"
)

func main() {
	toolrun.Main("shuf")
}
