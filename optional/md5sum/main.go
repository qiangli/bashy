package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/md5sum"
)

func main() {
	toolrun.Main("md5sum")
}
