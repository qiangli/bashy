package main

import (
	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/cmds/fetch"
)

func main() {
	adapter.RunToolMain("fetch")
}
