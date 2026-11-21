package main

import (
	"os"

	"github.com/qiangli/bashy/custom/internal/toolshortcut"
)

func main() {
	os.Exit(toolshortcut.RunToolShortcut("muse", os.Args[1:]))
}
