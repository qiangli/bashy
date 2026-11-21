package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/yoke/cmds/tar"
)

func main() {
	toolrun.Main("tar")
}
