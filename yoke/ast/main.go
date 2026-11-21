package main

import (
	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/cmds/ast"
)

func main() {
	adapter.RunToolMain("ast")
}
