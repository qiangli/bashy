package main

import (
	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/cmds/browser"
)

func main() {
	adapter.RunToolMain("browser")
}
