package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/base32"
)

func main() {
	toolrun.Main("base32")
}
