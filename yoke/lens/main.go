package main

import (
	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/cmds/lens"
)

func main() {
	adapter.RunToolMain("lens")
}
