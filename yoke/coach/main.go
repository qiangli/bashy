package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/chat"
)

func main() {
	os.Exit(adapter.RunCobra(chat.NewCoachCmd(), "", nil))
}
