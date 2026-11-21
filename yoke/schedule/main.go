package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/coreutils/pkg/schedule"
)

func main() {
	os.Exit(adapter.RunCobra(schedule.NewScheduleCmd(), "", nil))
}
