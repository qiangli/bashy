package agentos

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/qiangli/yoke/pkg/src"
)

// srcRepoProvenance is one fetched repo's provenance: the State tier
// vocabulary plus the two pins it was derived from. It mirrors
// src.RepoState with the repo name attached, so doctor rows and the fleet
// registry record carry the same fields.
type srcRepoProvenance struct {
	Repo  string `json:"repo"`
	State string `json:"state"`
	Base  string `json:"base"`
	Local string `json:"local"`
}

// fetchedSrcRepos lists the repos actually checked out under
// $BASHY_HOME/src: directories with a .git entry, skipping the dot-dirs
// (.prs, .approvals, .builds, .run) that hold src bookkeeping rather than
// repos. Sorted for deterministic output.
func fetchedSrcRepos(home string) []string {
	entries, err := os.ReadDir(src.SrcDir(home))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if st, err := os.Stat(filepath.Join(src.SrcDir(home), e.Name(), ".git")); err == nil && st.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// srcProvenance reports every fetched repo's State. It always computes the
// state through src.State (pristine versus local diff): a repo whose state
// cannot be determined is reported as unknown, never as pristine.
func srcProvenance(home string) []srcRepoProvenance {
	var out []srcRepoProvenance
	for _, repo := range fetchedSrcRepos(home) {
		st, err := src.State(home, repo)
		if err != nil {
			out = append(out, srcRepoProvenance{Repo: repo, State: "unknown"})
			continue
		}
		out = append(out, srcRepoProvenance{Repo: repo, State: st.State, Base: st.Base, Local: st.Local})
	}
	return out
}

// aggregateSrcState folds per-repo tiers into one host verdict:
// modified-protected wins over modified wins over pristine. Unknown only
// wins when nothing stronger was observed — it never masks a divergence,
// and pristine is reported only when every fetched repo checked pristine.
func aggregateSrcState(rows []srcRepoProvenance) string {
	state := src.StatePristine
	sawUnknown := false
	for _, r := range rows {
		switch r.State {
		case src.StateModifiedProtected:
			return src.StateModifiedProtected
		case src.StateModified:
			state = src.StateModified
		case src.StatePristine:
		default:
			sawUnknown = true
		}
	}
	if state == src.StatePristine && sawUnknown {
		return "unknown"
	}
	return state
}

// shortSHA12 abbreviates a commit SHA for human output. Full SHAs stay in
// JSON; human lines carry no hostnames, user ids, or cloudbox internals.
func shortSHA12(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
