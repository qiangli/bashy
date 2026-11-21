// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"strings"
	"testing"
	"time"
)

func TestRefuseSelfDowngrade(t *testing.T) {
	newer := time.Date(2026, 11, 21, 23, 0, 0, 0, time.UTC)
	older := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	stamps := map[string]time.Time{"installed-new": newer, "installed-old": older, "release-old": older, "release-new": newer}
	orig := selfCommitTime
	selfCommitTime = func(p string) (time.Time, bool) { v, ok := stamps[p]; return v, ok }
	t.Cleanup(func() { selfCommitTime = orig })
	t.Setenv(selfAllowDowngradeEnv, "")

	if err := refuseSelfDowngrade("installed-new", "release-old", false); err == nil || !strings.Contains(err.Error(), "refusing to downgrade") {
		t.Fatalf("newer installed build over an older release: want refusal, got %v", err)
	}
	if err := refuseSelfDowngrade("installed-new", "release-old", true); err != nil {
		t.Fatalf("--allow-downgrade must permit it: %v", err)
	}
	t.Setenv(selfAllowDowngradeEnv, "1")
	if err := refuseSelfDowngrade("installed-new", "release-old", false); err != nil {
		t.Fatalf("%s=1 must permit it: %v", selfAllowDowngradeEnv, err)
	}
	t.Setenv(selfAllowDowngradeEnv, "")
	for _, c := range [][2]string{{"installed-old", "release-new"}, {"installed-new", "release-new"}, {"missing", "release-old"}, {"installed-new", "unstamped"}} {
		if err := refuseSelfDowngrade(c[0], c[1], false); err != nil {
			t.Fatalf("%s <- %s must be allowed (upgrade, same, or unknown stamp): %v", c[0], c[1], err)
		}
	}
}
