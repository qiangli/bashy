// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"debug/buildinfo"
	"fmt"
	"os"
	"time"
)

// selfAllowDowngradeEnv lets an operator knowingly install an older bashy
// over a newer one (the flag --allow-downgrade does the same).
const selfAllowDowngradeEnv = "BASHY_SELF_ALLOW_DOWNGRADE"

// selfCommitTime is binaryCommitTime; tests replace it.
var selfCommitTime = binaryCommitTime

// binaryCommitTime reads the vcs.time stamp Go records in a binary built from
// a git tree. ok is false when the file is missing or carries no stamp.
func binaryCommitTime(path string) (time.Time, bool) {
	if _, err := os.Stat(path); err != nil {
		return time.Time{}, false
	}
	bi, err := buildinfo.ReadFile(path)
	if err != nil {
		return time.Time{}, false
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.time" {
			t, err := time.Parse(time.RFC3339, s.Value)
			return t, err == nil
		}
	}
	return time.Time{}, false
}

// refuseSelfDowngrade blocks a release install that would replace a newer
// installed bashy with an older one. On 2026-11-21 a service restore ran
// `bashy self install`, which fetched the latest promoted release (rc.37) and
// silently replaced a newer developer build, dropping agent records the newer
// build knew. Unknown stamps on either side never block: the check only fires
// on proof that the installed binary is newer.
func refuseSelfDowngrade(installed, incoming string, allow bool) error {
	if allow || os.Getenv(selfAllowDowngradeEnv) == "1" {
		return nil
	}
	have, okHave := selfCommitTime(installed)
	want, okWant := selfCommitTime(incoming)
	if !okHave || !okWant || !have.After(want) {
		return nil
	}
	return fmt.Errorf("refusing to downgrade %s: installed build is from %s, the release being installed is from %s.\n"+
		"Keep the newer build, or pass --allow-downgrade (or %s=1) to install the older release anyway",
		installed, have.UTC().Format(time.RFC3339), want.UTC().Format(time.RFC3339), selfAllowDowngradeEnv)
}
