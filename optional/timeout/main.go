package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/timeout"
)

func main() {
	toolrun.Main("timeout")
}
