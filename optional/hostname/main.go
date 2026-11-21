package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/hostname"
)

func main() {
	toolrun.Main("hostname")
}
