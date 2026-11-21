package main

import (
	"os"

	"github.com/qiangli/bashy/custom/internal/toolshortcut"
)

func main() {
	os.Exit(toolshortcut.RunToolShortcut("agy", os.Args[1:]))
}
