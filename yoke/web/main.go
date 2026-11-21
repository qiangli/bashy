package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/webinspect"
)

func main() {
	os.Exit(adapter.RunCobra(webinspect.NewWebCmd(), "", nil))
}
