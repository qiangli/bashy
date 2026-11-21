package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/sota"
)

func main() {
	os.Exit(adapter.RunCobra(sota.NewSotaCmd(), "", nil))
}
