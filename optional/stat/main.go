package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/stat"
)

func main() {
	toolrun.Main("stat")
}
