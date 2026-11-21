package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/numfmt"
)

func main() {
	toolrun.Main("numfmt")
}
