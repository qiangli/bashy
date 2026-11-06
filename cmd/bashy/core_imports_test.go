//go:build bashy_core || bashy_cert_base

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// coreImportAllowlist freezes today's offenders: it may only shrink. A new
// banned import fails TestCoreImportBoundary; a stale entry fails too.
var coreImportAllowlist = map[string]bool{
	// S1 49e058e0ae70 removes the yoke edges from the core/cert base.
	// PRE-EXISTING: yoke/pkg/telemetry arrives via internal/core -> yoke/pkg/fleet; it was
	// already banned and failing before this ratchet, so it is frozen here until S1.
	"github.com/qiangli/yoke/pkg/telemetry":             true,
	"github.com/qiangli/yoke/git":                       true,
	"github.com/qiangli/yoke/pkg/assetring":             true,
	"github.com/qiangli/yoke/pkg/atlas":                 true,
	"github.com/qiangli/yoke/pkg/prune":                 true,
	"github.com/qiangli/yoke/pkg/binmgr":                true,
	"github.com/qiangli/yoke/pkg/broker/door":           true,
	"github.com/qiangli/yoke/pkg/provider":              true,
	"github.com/qiangli/yoke/pkg/provider/aws":          true,
	"github.com/qiangli/yoke/pkg/provider/digitalocean": true,
	"github.com/qiangli/yoke/pkg/provider/enroll":       true,
	"github.com/qiangli/yoke/pkg/provider/scaffold":     true,
	"github.com/qiangli/yoke/pkg/ref":                   true,
	"github.com/qiangli/yoke/pkg/stateaccess":           true,
	"github.com/qiangli/yoke/pkg/room":                  true,
	"github.com/qiangli/yoke/pkg/secrets":               true,
	"github.com/qiangli/yoke/pkg/shape/appserver":       true,
	"github.com/qiangli/yoke/pkg/snippet":               true,
	"github.com/qiangli/yoke/pkg/bus/event":             true,
	"github.com/qiangli/yoke/pkg/spacetime":             true,
	"github.com/qiangli/yoke/pkg/fleet":                 true,
	// S2 18b0e41f259d removes the bashsharp/transpile edge.
	"github.com/bashsharp/bashsharp/transpile": true,
}

func TestCoreImportBoundary(t *testing.T) {
	for _, tags := range []string{"bashy_core", "bashy_cert_base"} {
		t.Run(tags, func(t *testing.T) {
			cmd := exec.Command("go", "list", "-deps", "-tags", tags, "./cmd/bashy")
			cmd.Dir = filepath.Join("..", "..")
			cmd.Env = os.Environ()
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("list core imports: %v\n%s", err, out)
			}
			seen := map[string]bool{}
			for _, pkg := range strings.Fields(string(out)) {
				seen[pkg] = true
				for _, forbidden := range []string{
					"github.com/qiangli/bashy/internal/agentos",
					"github.com/qiangli/ycode",
					"github.com/qiangli/filebrowser",
					"github.com/qiangli/yoke",
					"github.com/bashsharp/bashsharp/transpile",
					"modernc.org/sqlite",
				} {
					if (pkg == forbidden || strings.HasPrefix(pkg, forbidden+"/")) && !coreImportAllowlist[pkg] {
						t.Errorf("%s import graph contains optional package %s", tags, pkg)
					}
				}
			}
			for pkg := range coreImportAllowlist {
				if !seen[pkg] {
					t.Errorf("%s allowlist entry %s is no longer imported; delete it (allowlist is shrink-only)", tags, pkg)
				}
			}
		})
	}
}
