package agentos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	goversion "go/version"

	"github.com/qiangli/yoke/external/gotoolchain"
)

// ensureGoForTranspile makes a Go >= 1.27 toolchain reachable for the Go front
// end before `bashy transpile` runs: an explicit BASHPP_GO wins untouched; a
// host go that already meets the baseline is left to the SDK resolver's own
// PATH candidate; otherwise bashy's provisioned toolchain (the same one
// `bashy go` runs, digest-pinned via binmgr) is ensured and injected through
// BASHPP_GO, which the sh resolvers try first. On a stock Windows runner the
// host go is below the baseline, and without this the front end dies at
// "resolve Go bootstrap" instead of using bashy's own toolchain.
//
// A provisioning failure is reported on one line and otherwise ignored: the
// front end's own resolver states exactly what it could not find.
func ensureGoForTranspile(ctx context.Context) {
	if strings.TrimSpace(os.Getenv("BASHPP_GO")) != "" {
		return
	}
	if hostGoMeetsBaseline() {
		return
	}
	goBin, _, err := gotoolchain.Ensure(ctx, "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy transpile: provision go toolchain:", err)
		return
	}
	os.Setenv("BASHPP_GO", goBin)
}

// hostGoMeetsBaseline reports whether the PATH go, if any, is a Go >= 1.27
// toolchain the front end can use as it stands.
func hostGoMeetsBaseline() bool {
	hostGo, err := exec.LookPath("go")
	if err != nil {
		return false
	}
	out, err := exec.Command(hostGo, "env", "GOVERSION").Output()
	if err != nil {
		return false
	}
	return goVersionMeetsBaseline(strings.TrimSpace(string(out)))
}

// goVersionMeetsBaseline is the Go front end's floor; an unparseable version
// (a devel build) does not meet it.
func goVersionMeetsBaseline(reported string) bool {
	return goversion.IsValid(reported) && goversion.Compare(reported, "go1.27.0") >= 0
}
