package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/todo"
	_ "github.com/qiangli/yoke/pkg/weave"
)

func main() {
	os.Exit(adapter.RunCobra(todo.NewTodoCmd(), "bashy todo:", nil))
}
