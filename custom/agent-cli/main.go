package main

import (
	"os"

	"github.com/qiangli/bashy/custom/internal/toolshortcut"
)

func main() {
	os.Exit(toolshortcut.RunToolShortcut("", os.Args[1:]))
}
