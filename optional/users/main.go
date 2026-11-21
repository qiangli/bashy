package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/users"
)

func main() {
	toolrun.Main("users")
}
