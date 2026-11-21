package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/handoff"
)

func main() {
	os.Exit(adapter.RunCobra(handoff.NewResumeCmd(), "bashy resume:", nil))
}
