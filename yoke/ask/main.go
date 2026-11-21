package main

import (
	"fmt"
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/ask"
)

func main() {
	cmd := ask.NewAskCmd()
	os.Exit(adapter.RunCobra(cmd, "", func(err error) int {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}))
}
