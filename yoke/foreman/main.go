package main

import (
	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/cmds/foreman"
)

func main() {
	adapter.RunToolMain("foreman")
}
