package agentos

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/bashsharp/bashsharp/front"
)

// TestGoSourceFrontEndIsLinkedByDefault is the whole point of removing the
// build tag: a default `go build ./cmd/bashy` must carry the front end, so
// `--source=go` can never be answered with "not available in this build".
func TestGoSourceFrontEndIsLinkedByDefault(t *testing.T) {
	if front.GoSourceLoad == nil {
		t.Fatal("front.GoSourceLoad is nil: the Go front end is not linked")
	}
	if front.GoSourcePackageFiles == nil {
		t.Fatal("front.GoSourcePackageFiles is nil: directory recipes cannot be selected")
	}
}

// TestGoSourceFrontEndIsNotLinkedIntoClassicBash pins the boundary this
// package's doc comment claims, and CORRECTS the older, now-false half of it.
//
// Still true: the Go source FRONT END (mvdan.cc/sh/v3/gosource plus this
// wiring) is reachable only from cmd/bashy.
//
// No longer true: "cmd/bash links no go/types". sh de4ff069's Bash++ native
// bridge (interp/bashpp_native_bridge.go) imports go/types and go/importer in
// package interp, which BOTH binaries link. The test asserts that as an
// observed fact so the next reader does not "fix" the comment back.
func TestGoSourceFrontEndIsNotLinkedIntoClassicBash(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps",
		"github.com/qiangli/bashy/cmd/bash").CombinedOutput()
	if err != nil {
		t.Skipf("go list unavailable (%v): %s", err, out)
	}
	deps := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		deps[strings.TrimSpace(line)] = true
	}
	for _, pkg := range []string{
		"mvdan.cc/sh/v3/gosource",
		"github.com/qiangli/bashy/internal/agentos",
	} {
		if deps[pkg] {
			t.Errorf("cmd/bash links %s — the Go front end must stay in the AgentOS half", pkg)
		}
	}
	// The corrected claim, asserted rather than asserted-away.
	if !deps["go/types"] {
		t.Error("cmd/bash no longer links go/types: the doc comment in gosource.go " +
			"is stale again and should say the drop-in links no type checker")
	}
}
