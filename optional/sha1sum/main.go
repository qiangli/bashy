package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/sha1sum"
)

func main() {
	toolrun.Main("sha1sum")
}
