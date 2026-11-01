package agentos

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestSrcPinsMatchLinkedModules(t *testing.T) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("linked build metadata missing")
	}
	for _, repo := range []string{"yoke", "coreutils", "sh", "filebrowser"} {
		t.Run(repo, func(t *testing.T) {
			want := srcModulePins[repo]
			var linked *debug.Module
			for _, dep := range bi.Deps {
				if dep.Path == want.module || (want.upstream != "" && dep.Path == want.upstream) {
					linked = dep
					break
				}
			}
			if linked == nil {
				t.Fatal("expected source dependency absent from actual build metadata")
			}
			for linked.Replace != nil {
				linked = linked.Replace
			}
			pin, err := srcPinnedCommit(repo)
			if linked.Version == "" || linked.Version == "(devel)" {
				if pin != "" || err == nil || !strings.Contains(err.Error(), "unversioned local module") {
					t.Fatalf("local build must refuse release pin: pin=%q err=%v", pin, err)
				}
				return
			}
			if err != nil || pin != want.commit {
				t.Fatalf("published build pin=%q err=%v want=%s", pin, err, want.commit)
			}
		})
	}
	if pin, err := srcPinnedCommit("managed"); err != nil || pin != "unknown" {
		t.Errorf("managed external pin = %q, %v", pin, err)
	}
}

func TestSrcPinBuildInfoContract(t *testing.T) {
	for repo, pin := range srcModulePins {
		t.Run(repo, func(t *testing.T) {
			published := "v0.0.0-20261001000000-" + pin.commit[:12]
			upstream := pin.upstream
			if upstream == "" {
				upstream = pin.module
			}
			cases := []struct {
				name      string
				dep       *debug.Module
				wantError string
			}{
				{"published", &debug.Module{Path: pin.module, Version: published}, ""},
				{"published replacement", &debug.Module{Path: upstream, Version: "v1.0.0", Replace: &debug.Module{Path: pin.module, Version: published}}, ""},
				{"wrong revision", &debug.Module{Path: pin.module, Version: "v0.0.0-20261001000000-deadbeef0000"}, "out of sync"},
				{"workspace", &debug.Module{Path: upstream, Version: "(devel)"}, "unversioned local module"},
				{"local replacement", &debug.Module{Path: upstream, Version: published, Replace: &debug.Module{Path: "../local"}}, "unversioned local module"},
				{"pinned dependency replaced locally", &debug.Module{Path: pin.module, Version: published, Replace: &debug.Module{Path: "../local"}}, "unversioned local module"},
				{"other fork with matching revision", &debug.Module{Path: upstream, Version: published, Replace: &debug.Module{Path: "example.com/other", Version: published}}, "not the pinned module"},
				{"absent", &debug.Module{Path: "example.com/absent", Version: published}, "absent"},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					got, err := srcPinnedCommitFromBuildInfo(repo, &debug.BuildInfo{Deps: []*debug.Module{tc.dep}})
					if tc.wantError == "" {
						if err != nil || got != pin.commit {
							t.Fatalf("pin=%q err=%v want=%s", got, err, pin.commit)
						}
					} else if got != "" || err == nil || !strings.Contains(err.Error(), tc.wantError) {
						t.Fatalf("pin=%q err=%v want error containing %q", got, err, tc.wantError)
					}
				})
			}
		})
	}
	if got, err := srcPinnedCommitFromBuildInfo("yoke", nil); got != "" || err == nil {
		t.Fatalf("missing build info accepted: %q %v", got, err)
	}
}
