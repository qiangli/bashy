//go:build bashy_core

// The bashy_core profile is an architecture probe. It keeps one physical
// executable with shell, Coreutils, and registered-command CRUD while optional
// AgentOS/ycode/Genie packages are absent from its Go import graph.
package main

import (
	"fmt"
	"os"

	// The Bash# Go-source loader is part of the language base. It registers
	// front.GoSourceLoad without importing the optional AgentOS graph.
	_ "github.com/bashsharp/bashsharp/transpile"
	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/bashy/internal/core"
	_ "github.com/qiangli/coreutils/cmds/all"
	"github.com/qiangli/coreutils/multicall"
	"github.com/qiangli/coreutils/tool"
	_ "golang.org/x/crypto/x509roots/fallback"
	"mvdan.cc/sh/v3/interp/ownedexec"
)

func init() {
	if shellInvocation(os.Args[0]) || utilityInvocation(os.Args[0]) {
		return
	}
	cli.AgentOSOwnedCommand = func(name string) bool { return tool.Lookup(name) != nil }
	cli.AgentOSOwnedNames = tool.Names
	cli.AgentOSDispatch = core.Dispatch
	cli.AgentOSWireExec = core.WireExec
	cli.AgentOSBashPPDefault = true
	cli.VersionProduct = "bashy"
	cli.VersionCompatibility = "GNU Bash 5.3 compatible"
	cli.SuppressedForkBuiltins = nil
}

func main() {
	adoptingOwnedFrame := len(os.Args) == 2 && os.Args[1] == ownedexec.Sentinel
	if err := ownedexec.Adopt(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(126)
	}
	if adoptingOwnedFrame {
		cli.AdoptGoSourceProcessEnvironment()
	}
	if utilityInvocation(os.Args[0]) {
		multicall.Main("coreutils")
		return
	}
	if shellInvocation(os.Args[0]) {
		installInheritedSignalIgnores()
		cli.Main()
		return
	}
	cli.MaybeRunJobCarrierHelper()
	installInheritedSignalIgnores()
	cli.Main()
}
