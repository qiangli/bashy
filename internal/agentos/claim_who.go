package agentos

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gitobj "github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"

	outgit "github.com/qiangli/yoke/git"
	"github.com/qiangli/yoke/pkg/policy/coord"
	"github.com/qiangli/yoke/pkg/room"
	"github.com/qiangli/yoke/pkg/weave"
)

// claim_who.go — "who is holding this path, and how do I reach them?"
//
// The question this answers is the one an agent asks a second before it edits
// somebody else's file. Every piece of the answer already existed on this host,
// in four different stores that never met: the coord ledger (an explicit hold),
// the room board (a live session's cwd), the weave queue (a dispatched run), and
// the local git history (committed, not yet pushed — the only record of an agent
// that has already finished writing). So this command STORES NOTHING: it is a
// join over what is already true, which is why a holder it reports cannot be
// stale in a way the owning store is not.
//
// It never guesses. An unowned path is reported as unowned, with the board post
// that finds an owner — an invented holder is worse than none, because it sends
// the asker to coordinate with somebody who was never there.

const claimWhoSchema = "bashy-claim-who-v1"

// The four evidence sources, in the order they are reported: an explicit hold
// outranks a live cwd, which outranks a dispatched run, which outranks history.
const (
	claimWhoSourceClaim  = "claim"
	claimWhoSourceRoom   = "room"
	claimWhoSourceWeave  = "weave"
	claimWhoSourceCommit = "commit"
)

// claimWhoScanLimit bounds the unpushed-history walk. @{u}..HEAD is normally a
// handful of commits; a detached or long-lived branch must not turn a question
// into a full history traversal.
const claimWhoScanLimit = 200

// claimWhoOwner is one holder, from one source, with the evidence that put it
// there. Liveness is the owning store's verdict, never this command's guess:
// "unknown" is a real answer and is reported as one.
type claimWhoOwner struct {
	Holder   string `json:"holder"`
	Source   string `json:"source"`
	Intent   string `json:"intent,omitempty"`
	PID      int    `json:"pid,omitempty"`
	Liveness string `json:"liveness"`
	Age      string `json:"age,omitempty"`
	Evidence string `json:"evidence,omitempty"`
	Sprint   string `json:"sprint,omitempty"`
	StoryID  string `json:"story_id,omitempty"`
	Reach    string `json:"reach,omitempty"`
}

type claimWhoPath struct {
	Path   string          `json:"path"`
	Abs    string          `json:"abs,omitempty"`
	Owners []claimWhoOwner `json:"owners"`
	Advice string          `json:"advice,omitempty"`
}

type claimWhoReport struct {
	SchemaVersion string         `json:"schema_version"`
	Paths         []claimWhoPath `json:"paths"`
	// Warnings names a store that could not be read, so an empty answer is
	// never mistaken for "nobody holds it".
	Warnings []string `json:"warnings,omitempty"`
	// HiddenStale counts lapsed/vacant claims left out (see --all).
	HiddenStale int `json:"hidden_stale,omitempty"`
}

// claimWhoOptions are the seams: tests point them at throwaway state, and the
// command leaves them zero so every store resolves exactly as it does for the
// store's own commands.
type claimWhoOptions struct {
	cwd            string
	coordDir       string
	weaveStateRoot string
	now            time.Time
	// all keeps lapsed/vacant claims. By default they are hidden: a claim a
	// successor may take without consent is history, not a holder to contact,
	// and a busy host carries hundreds of them.
	all bool
}

// claimWhoCommit is one committed-but-unpushed commit, with the files it
// touched and the trailers that say who wrote it.
type claimWhoCommit struct {
	hash    string
	holder  string
	subject string
	sprint  string
	storyID string
	when    time.Time
	files   []string
}

func newClaimWhoCmd() *cobra.Command {
	var asJSON, all bool
	cmd := &cobra.Command{
		Use:   "who [PATH...]",
		Short: "who is holding these paths, and how to reach them",
		Long: `who answers the question asked one second before you edit somebody else's file.

It composes four records this host already keeps and nothing joined before:

  claim    an explicit hold in the coord ledger whose path set covers the path
  room     a LIVE session whose cwd contains the path
  weave    a dispatched weave run working the path's repo
  commit   a committed-but-unpushed commit (@{u}..HEAD) that touched the path,
           attributed by its Agent: / Story-ID: / Sprint: trailers

With no PATH it asks about the dirty paths of this checkout — the files you are
about to collide over.

It stores nothing and it never guesses. A path nobody holds is reported as
unowned, with the board post that finds the owner; a holder whose liveness no
store can vouch for is reported "unknown" rather than assumed gone.`,
		Example: `  bashy claim who                      # the dirty paths of this checkout
  bashy claim who internal/cli docs    # specific paths
  bashy claim who --json internal/cli  # the bashy-claim-who-v1 document`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rep, err := claimWhoReportFor(args, claimWhoOptions{all: all})
			if err != nil {
				return err
			}
			if asJSON {
				return renderClaimWhoJSON(cmd.OutOrStdout(), rep)
			}
			return renderClaimWho(cmd.OutOrStdout(), rep)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the "+claimWhoSchema+" document")
	cmd.Flags().BoolVar(&all, "all", false, "also list lapsed/vacant claims (hidden by default: nobody to contact)")
	return cmd
}

// claimWhoReportFor reads every store ONCE and then answers each path from that
// one snapshot: two paths in the same answer can never disagree about who is
// working here.
func claimWhoReportFor(args []string, opts claimWhoOptions) (claimWhoReport, error) {
	cwd := opts.cwd
	if cwd == "" {
		var err error
		if cwd, err = os.Getwd(); err != nil {
			return claimWhoReport{}, err
		}
	}
	now := opts.now
	if now.IsZero() {
		now = time.Now()
	}
	rep := claimWhoReport{SchemaVersion: claimWhoSchema, Paths: []claimWhoPath{}}

	targets, err := claimWhoTargets(args, cwd)
	if err != nil {
		return claimWhoReport{}, err
	}

	coordDir := opts.coordDir
	if coordDir == "" {
		coordDir = coord.DefaultDir()
	}
	claims, err := coord.List(coordDir)
	if err != nil {
		rep.Warnings = append(rep.Warnings, "coord ledger unreadable: "+err.Error())
	}
	members, err := room.PeekMembers()
	if err != nil {
		rep.Warnings = append(rep.Warnings, "room board unreadable: "+err.Error())
	}
	// A room card is sorted deepest-cwd-first ONCE: the closest working
	// directory is the most specific claim on the path.
	sort.SliceStable(members, func(i, j int) bool {
		return claimWhoDepth(members[i].Cwd) > claimWhoDepth(members[j].Cwd)
	})

	holdersByRepo := map[string][]weave.Holder{}
	commitsByRepo := map[string][]claimWhoCommit{}
	for _, t := range targets {
		// RepoRootOf walks UP from the path, so it answers for a file and for a
		// directory alike — including a repo root named directly, whose own
		// parent may belong to a different checkout.
		repoRoot := weave.RepoRootOf(t.abs)
		entry := claimWhoPath{Path: t.display, Abs: t.abs, Owners: []claimWhoOwner{}}

		for _, c := range claims {
			if c == nil {
				continue
			}
			scope := append(append([]string{}, c.Roots...), c.Members...)
			if !coord.Intersects(scope, []string{t.abs}) {
				continue
			}
			if !opts.all && c.Stale(now) {
				rep.HiddenStale++
				continue
			}
			entry.Owners = append(entry.Owners, claimWhoClaimOwner(c, t.abs, now))
		}
		for _, card := range members {
			if !claimWhoWithin(card.Cwd, t.abs) {
				continue
			}
			entry.Owners = append(entry.Owners, claimWhoRoomOwner(card, now))
		}
		if repoRoot != "" {
			holders, ok := holdersByRepo[repoRoot]
			if !ok {
				holders = weave.HoldersOf(repoRoot, weave.HoldersQuery{StateRoot: opts.weaveStateRoot})
				holdersByRepo[repoRoot] = holders
			}
			for _, h := range holders {
				entry.Owners = append(entry.Owners, claimWhoWeaveOwner(h))
			}

			commits, ok := commitsByRepo[repoRoot]
			if !ok {
				commits = claimWhoUnpushedCommits(repoRoot)
				commitsByRepo[repoRoot] = commits
			}
			for _, c := range commits {
				if !claimWhoCommitTouches(c, t.abs) {
					continue
				}
				entry.Owners = append(entry.Owners, claimWhoCommitOwner(c, now))
			}
		}
		if len(entry.Owners) == 0 {
			entry.Advice = fmt.Sprintf(
				"no owner found; post bashy mb post --topic announce %q",
				"taking "+t.display+" — speak up if you hold it")
		}
		rep.Paths = append(rep.Paths, entry)
	}
	return rep, nil
}

type claimWhoTarget struct {
	display string
	abs     string
}

// claimWhoTargets resolves the paths to ask about. With no argument the answer
// is this checkout's DIRTY paths: the files already being edited here are
// exactly the ones a collision is about.
func claimWhoTargets(args []string, cwd string) ([]claimWhoTarget, error) {
	if len(args) > 0 {
		out := make([]claimWhoTarget, 0, len(args))
		for _, arg := range args {
			abs := arg
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(cwd, abs)
			}
			out = append(out, claimWhoTarget{display: arg, abs: filepath.Clean(abs)})
		}
		return out, nil
	}
	repoRoot := weave.RepoRootOf(cwd)
	if repoRoot == "" {
		return nil, errors.New("claim who: not a git checkout here — name the paths to ask about")
	}
	_, entries, err := outgit.Status(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("claim who: read working tree: %w", err)
	}
	seen := map[string]bool{}
	var files []string
	for _, e := range entries {
		file := strings.TrimSpace(e.File)
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		files = append(files, file)
	}
	sort.Strings(files)
	out := make([]claimWhoTarget, 0, len(files))
	for _, file := range files {
		out = append(out, claimWhoTarget{display: file, abs: filepath.Join(repoRoot, file)})
	}
	return out, nil
}

func claimWhoClaimOwner(c *coord.Claim, target string, now time.Time) claimWhoOwner {
	scope := c.Resource
	if scope == "" && len(c.Roots) > 0 {
		// Name only the root that covers this path: a project claim lists every
		// sibling it depends on, and the full set buries the one that matters.
		scope = claimWhoMatchingRoot(c.Roots, target)
	}
	age := assignmentAgeAt(c.Heartbeat, now)
	if age == "" {
		age = assignmentAgeAt(c.AcquiredAt, now)
	}
	return claimWhoOwner{
		Holder:   c.Holder.Name,
		Source:   claimWhoSourceClaim,
		Intent:   c.Intent,
		PID:      c.PID,
		Liveness: string(c.Liveness(now)),
		Age:      age,
		Evidence: "claim on " + scope,
		Reach:    claimWhoReach(c.Holder.Name),
	}
}

func claimWhoRoomOwner(card room.Card, now time.Time) claimWhoOwner {
	joined := parseRoomJoined(card.Joined)
	// The room prunes a card whose holder process is gone, so a card read here
	// is live by construction — but say so from the pid, not from the read.
	liveness := "unknown"
	if card.PID > 0 && room.PidAlive(card.PID) {
		liveness = "live"
	}
	return claimWhoOwner{
		Holder:   roomAgentName(card),
		Source:   claimWhoSourceRoom,
		Intent:   strings.TrimSpace(card.Task),
		PID:      card.PID,
		Liveness: liveness,
		Age:      assignmentAgeAt(joined, now),
		Evidence: "cwd " + card.Cwd,
		Reach:    claimWhoReach(roomAgentName(card)),
	}
}

func claimWhoWeaveOwner(h weave.Holder) claimWhoOwner {
	holder := strings.TrimSpace(h.Tool)
	if holder == "" {
		holder = fmt.Sprintf("weave-run-%d", h.ID)
	}
	evidence := fmt.Sprintf("weave run #%d (%s)", h.ID, h.State)
	if h.Workspace != "" {
		evidence += " in " + h.Workspace
	}
	return claimWhoOwner{
		Holder: holder,
		Source: claimWhoSourceWeave,
		Intent: strings.TrimSpace(h.Title),
		// A queue records the run, not a process: the run's liveness belongs to
		// weave, and nothing readable here proves it.
		Liveness: "unknown",
		Evidence: evidence,
		Reach:    claimWhoReach(holder),
	}
}

func claimWhoCommitOwner(c claimWhoCommit, now time.Time) claimWhoOwner {
	return claimWhoOwner{
		Holder:   c.holder,
		Source:   claimWhoSourceCommit,
		Intent:   c.subject,
		Liveness: "unknown",
		Age:      assignmentAgeAt(c.when, now),
		Evidence: "unpushed commit " + c.hash,
		Sprint:   c.sprint,
		StoryID:  c.storyID,
		Reach:    claimWhoReach(c.holder),
	}
}

func claimWhoReach(holder string) string {
	holder = strings.TrimSpace(holder)
	if holder == "" {
		return ""
	}
	return fmt.Sprintf("bashy mb send %s %q", holder, "...")
}

// claimWhoWithin reports whether dir CONTAINS p. One-directional on purpose: a
// session working in a subdirectory is not evidence about its parent.
func claimWhoWithin(dir, p string) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" || p == "" {
		return false
	}
	return coord.Intersects([]string{dir}, []string{p}) && claimWhoDepth(dir) <= claimWhoDepth(p)
}

func claimWhoDepth(p string) int {
	p = strings.TrimSpace(p)
	if p == "" {
		return -1
	}
	return len(strings.Split(filepath.Clean(p), string(filepath.Separator)))
}

func claimWhoCommitTouches(c claimWhoCommit, abs string) bool {
	for _, f := range c.files {
		if coord.Intersects([]string{f}, []string{abs}) {
			return true
		}
	}
	return false
}

// claimWhoUnpushedCommits reads @{u}..HEAD: commits that exist only here.
//
// Absence of evidence is not an error. A repo with no upstream, a detached
// HEAD, or a branch that was never pushed simply has nothing bounded to report,
// and this command must stay usable in all three.
func claimWhoUnpushedCommits(repoRoot string) []claimWhoCommit {
	r, err := gogit.PlainOpenWithOptions(repoRoot, &gogit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil
	}
	head, err := r.Head()
	if err != nil || !head.Name().IsBranch() {
		return nil
	}
	cfg, err := r.Config()
	if err != nil {
		return nil
	}
	branch := cfg.Branches[head.Name().Short()]
	if branch == nil || branch.Remote == "" || branch.Merge == "" {
		return nil
	}
	upstream, err := r.Reference(plumbing.NewRemoteReferenceName(branch.Remote, branch.Merge.Short()), true)
	if err != nil {
		return nil
	}
	iter, err := r.Log(&gogit.LogOptions{From: head.Hash()})
	if err != nil {
		return nil
	}
	defer iter.Close()

	var out []claimWhoCommit
	for i := 0; i < claimWhoScanLimit; i++ {
		c, err := iter.Next()
		if err != nil {
			break
		}
		if c.Hash == upstream.Hash() {
			break
		}
		out = append(out, claimWhoCommitOf(repoRoot, c))
	}
	return out
}

func claimWhoCommitOf(repoRoot string, c *gitobj.Commit) claimWhoCommit {
	agent, sprint, storyID := claimWhoTrailers(c.Message)
	holder := agent
	if holder == "" {
		// No Agent: trailer. The committer identity is the only attribution the
		// commit carries, so report THAT rather than inventing an agent name.
		holder = strings.TrimSpace(c.Author.Name)
	}
	out := claimWhoCommit{
		hash:    c.Hash.String()[:8],
		holder:  holder,
		subject: claimWhoSubject(c.Message),
		sprint:  sprint,
		storyID: storyID,
		when:    c.Committer.When,
	}
	stats, err := c.Stats()
	if err != nil {
		return out
	}
	for _, s := range stats {
		if s.Name == "" {
			continue
		}
		out.files = append(out.files, filepath.Join(repoRoot, s.Name))
	}
	return out
}

func claimWhoSubject(message string) string {
	line, _, _ := strings.Cut(message, "\n")
	return strings.TrimSpace(line)
}

// claimWhoTrailers reads the delivery trailers this project already writes on
// every commit: Agent:, Sprint:, Story-ID:.
func claimWhoTrailers(message string) (agent, sprint, storyID string) {
	for _, line := range strings.Split(message, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "agent":
			if agent == "" {
				agent = value
			}
		case "sprint":
			if sprint == "" {
				sprint = value
			}
		case "story-id":
			if storyID == "" {
				storyID = value
			}
		}
	}
	return agent, sprint, storyID
}

func renderClaimWhoJSON(w io.Writer, rep claimWhoReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func renderClaimWho(w io.Writer, rep claimWhoReport) error {
	for _, warning := range rep.Warnings {
		fmt.Fprintln(w, "claim who: "+warning)
	}
	if rep.HiddenStale > 0 {
		fmt.Fprintf(w, "claim who: %d lapsed claim(s) hidden (--all shows them)\n", rep.HiddenStale)
	}
	if len(rep.Paths) == 0 {
		fmt.Fprintln(w, "claim who: nothing dirty here — name a path to ask about one")
		return nil
	}
	for i, p := range rep.Paths {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, p.Path)
		if len(p.Owners) == 0 {
			fmt.Fprintln(w, "  "+p.Advice)
			continue
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, o := range p.Owners {
			pid := "-"
			if o.PID > 0 {
				pid = fmt.Sprintf("pid %d", o.PID)
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\n",
				o.Holder, o.Source, o.Liveness, pid, dash(o.Age), dash(claimWhoWhat(o)))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		// One reach line per HOLDER: the same agent listed from two sources is
		// still one agent to talk to.
		reached := map[string]bool{}
		for _, o := range p.Owners {
			if o.Reach == "" || reached[o.Holder] {
				continue
			}
			reached[o.Holder] = true
			fmt.Fprintln(w, "  reach "+o.Holder+": "+o.Reach)
		}
	}
	return nil
}

// claimWhoWhat is the one-line "what are they doing", with the evidence that
// says why this holder is listed at all.
func claimWhoWhat(o claimWhoOwner) string {
	parts := []string{}
	if o.Intent != "" {
		parts = append(parts, o.Intent)
	}
	var tags []string
	if o.Sprint != "" {
		tags = append(tags, "sprint "+o.Sprint)
	}
	if o.StoryID != "" {
		tags = append(tags, "story "+o.StoryID)
	}
	if o.Evidence != "" {
		tags = append(tags, o.Evidence)
	}
	if len(tags) > 0 {
		parts = append(parts, "["+strings.Join(tags, " · ")+"]")
	}
	return strings.Join(parts, " ")
}

// claimWhoMatchingRoot is the deepest root that contains target (or that
// target contains), else the first root.
func claimWhoMatchingRoot(roots []string, target string) string {
	best := ""
	for _, r := range roots {
		if (claimWhoWithin(r, target) || claimWhoWithin(target, r)) && len(r) > len(best) {
			best = r
		}
	}
	if best == "" && len(roots) > 0 {
		best = roots[0]
	}
	return best
}
