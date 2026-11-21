package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/accounts"
	_ "github.com/qiangli/yoke/pkg/accounts/builtin"
)

func main() {
	os.Exit(adapter.RunCobra(accounts.NewAccountCmd(), "", nil))
}
