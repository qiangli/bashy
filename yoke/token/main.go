package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/pkg/accounts/builtin"
	"github.com/qiangli/yoke/pkg/fleet"
)

func main() {
	os.Exit(adapter.RunCobra(fleet.NewTokenCmd(), "bashy token:", fleet.ExitCode))
}
