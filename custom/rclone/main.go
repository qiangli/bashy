package main

import (
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/rclone"
)

func main() {
	os.Exit(adapter.RunCobra(rclone.NewRcloneCmd(), "", nil))
}
