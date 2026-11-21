package main

import (
	"context"
	"os"

	"github.com/qiangli/yoke/pkg/herald"
)

func main() {
	os.Exit(herald.Run(context.Background(), append([]string{"acp"}, os.Args[1:]...)))
}
