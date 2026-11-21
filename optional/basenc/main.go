package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/basenc"
)

func main() {
	toolrun.Main("basenc")
}
