package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/mknod"
)

func main() {
	toolrun.Main("mknod")
}
