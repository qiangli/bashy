package main

import (
	"os"

	"github.com/qiangli/yoke/pkg/gomod"
)

func main() {
	os.Exit(gomod.Run(os.Args[1:], os.Stdout, os.Stderr))
}
