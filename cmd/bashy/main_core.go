//go:build bashy_core || bashy_cert_base

// bashy_core is an architecture probe; bashy_cert_base is the candidate
// base/core product. Both keep one physical executable with shell, Bash#,
// Coreutils, and command CRUD while optional AgentOS/ycode/Genie packages
// are absent from the process import graph.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	// The Bash# Go-source loader is part of the language base. It registers
	// front.GoSourceLoad without importing the optional AgentOS graph.
	_ "github.com/bashsharp/bashsharp/transpile"
	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/bashy/internal/core"
	_ "github.com/qiangli/coreutils/cmds/all"
	"github.com/qiangli/coreutils/multicall"
	_ "golang.org/x/crypto/x509roots/fallback"
	"mvdan.cc/sh/v3/interp/ownedexec"
)

// The base profile has no AgentOS execution handlers. Its dry-run request
// therefore validates source without executing it, including outside POSIX
// mode. Keep the flag in this entry point so the plain bash binary never
// acquires a Bashy-specific option.
var baseDryRun = flag.Bool("dryrun", false, "bashy: parse source without executing it")

func baseDryRunRequested() bool {
	if *baseDryRun {
		return true
	}
	// Preserve the startup safety request if an embedding caller resets the
	// process-global flag set after it parses the command line.
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--dry-run", "--dryrun":
			return true
		case "--", "-c":
			return false
		}
		if !strings.HasPrefix(arg, "-") {
			return false
		}
	}
	return false
}

func init() {
	flag.BoolVar(baseDryRun, "dry-run", false, "bashy: alias for --dryrun")
	if shellInvocation(os.Args[0]) || utilityInvocation(os.Args[0]) {
		return
	}
	cli.AgentOSCommandLineNoExec = func(bool) bool { return baseDryRunRequested() }
	cli.AgentOSStrictPosixParse = func(posix bool) bool { return posix && baseDryRunRequested() }
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
