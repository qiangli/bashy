package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/arch"
)

func main() {
	toolrun.Main("arch")
}
