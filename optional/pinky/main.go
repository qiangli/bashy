package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/pinky"
)

func main() {
	toolrun.Main("pinky")
}
