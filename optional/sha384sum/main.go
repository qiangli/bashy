package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/sha384sum"
)

func main() {
	toolrun.Main("sha384sum")
}
