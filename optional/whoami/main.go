package main

import (
	"github.com/qiangli/bashy/optional/internal/toolrun"
	_ "github.com/qiangli/coreutils/cmds/whoami"
)

func main() {
	toolrun.Main("whoami")
}
