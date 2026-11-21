package main

import (
	"os"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/seaweedfs"
)

func main() {
	os.Exit(adapter.RunCobra(seaweedfs.NewSeaweedfsCmd(), "", nil))
}
