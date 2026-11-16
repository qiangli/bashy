package agentos

import (
	"os"
	"strings"
	"testing"
)

func TestSrcPinsMatchLinkedModules(t *testing.T) {
	for _, repo := range []string{"yoke", "coreutils", "sh", "filebrowser"} {
		pin, err := srcPinnedCommit(repo)
		if err != nil {
			t.Errorf("%s: %v", repo, err)
			continue
		}
		if len(pin) != 40 || !strings.HasPrefix(pin, srcModulePins[repo].commit[:12]) {
			t.Errorf("%s pin = %q", repo, pin)
		}
	}
	if pin, err := srcPinnedCommit("managed"); err != nil || pin != "unknown" {
		t.Errorf("managed external pin = %q, %v", pin, err)
	}
}

// TestSrcPinsReplaceTheLinkedModulePath: `self build --local` points a Go
// replace at the local clone, so a fork published under an upstream path
// must name that path and its in-repo directory. A replace aimed at the
// fork's own path is inert and silently builds the release code.
func TestSrcPinsReplaceTheLinkedModulePath(t *testing.T) {
	mod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for repo, pin := range srcModulePins {
		if pin.replaces == "" {
			if strings.Contains(string(mod), "=> "+pin.module+" ") {
				t.Errorf("%s: go.mod replaces some module with %s; set the pin's replaces", repo, pin.module)
			}
			continue
		}
		if !strings.Contains(string(mod), "replace "+pin.replaces+" => "+pin.module+" ") {
			t.Errorf("%s: go.mod has no `replace %s => %s`", repo, pin.replaces, pin.module)
		}
	}
	if fb := srcModulePins["filebrowser"]; fb.subdir != "backend" {
		t.Errorf("filebrowser subdir = %q, want backend (the module lives in backend/)", fb.subdir)
	}
}
