package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/pkg/accounts/builtin"
	"github.com/qiangli/yoke/pkg/secrets"
)

func main() {
	name := adapter.InvokedAs("secret")
	cmd := secrets.NewSecretsCmd()
	cmd.Use = name
	os.Exit(adapter.RunCobra(cmd, name+":", nil))
}
