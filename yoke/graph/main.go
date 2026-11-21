package main

import (
	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/cmds/graph"
)

func main() {
	adapter.RunToolMain("graph")
}
