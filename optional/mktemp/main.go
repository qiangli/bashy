package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/mktemp"
)

func main() {
	toolrun.Main("mktemp")
}
