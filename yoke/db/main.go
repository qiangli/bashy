package main

import (
	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/cmds/db"
)

func main() {
	adapter.RunToolMain("db")
}
