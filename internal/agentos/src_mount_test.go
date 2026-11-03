package agentos

import (
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
