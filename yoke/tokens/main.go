package main

import (
	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/cmds/tokens"
)

func main() {
	adapter.RunToolMain("tokens")
}
