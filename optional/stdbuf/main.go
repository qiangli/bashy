package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/stdbuf"
)

func main() {
	toolrun.Main("stdbuf")
}
