package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/truncate"
)

func main() {
	toolrun.Main("truncate")
}
