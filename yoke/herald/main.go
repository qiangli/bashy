package main

import (
	"context"
	"os"

	"github.com/qiangli/yoke/pkg/herald"
)

func main() {
	os.Exit(herald.Run(context.Background(), os.Args[1:]))
}
