package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/link"
)

func main() {
	toolrun.Main("link")
}
