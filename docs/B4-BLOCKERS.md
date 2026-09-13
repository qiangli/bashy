# B4 blockers — `bashy define <skill>` cannot receive the skill rows without a lexicon seam

Sprint #161 · Story #262 · Story-ID 269f1dcf69b8

## What shipped

- `bashy inspect actions [--json] [--kind command|script|agent|skill]` — the
  facet catalog, generic half only, sorted by kind then identity
  (`internal/agentos/inspect_actions.go`).
- `bashy inspect context --json` → `actions: {command, script, agent, skill}`
  (script is honestly `0`; nothing projects that family yet).
- `bashy define <verb|agent>` already prints the facet (the `runs:` line, and
  the nested `action` object under `--json`) — pkg/lexicon renders it and
  `TestDefinePrintsActionFacet` now ratchets the bashy mount.
- `lexiconSkillRows()` — the `skills.Skill → lexicon.SkillRow` conversion at
  the bashy call site, exactly the shape `skillRowOf` in
  `../coreutils/pkg/lexicon/action_test.go` describes. `inspect actions` and
  the context counts already use it, so the skill facets are computed and
  correct on this host (`conductor` → `dhnt · judge · agentic · effects
  read,write,net,spend,time · via dhnt [h03af…]`).

## What is blocked, and why

The story asks that "the lexicon Store bashy builds for define also gets the
skills rows". **bashy does not build that store.** `lexicon.NewDefineCmd()`
builds it inside its own `RunE` via the unexported `buildFull`, and pkg/lexicon
exposes no seam for skills: `Synopses`, `KnownCommands`, `RecordDiscovery` and
`SeatSource` are the only injection points, and none can carry a `SkillRow`.
`AddSkills` exists (coreutils `6fd41652`) but nothing in lexicon's own build
path calls it — the commit says "the caller that has a catalog passes its rows
in", and there is nowhere for the caller to pass them.

The two ways to close it without the seam were rejected on purpose:

1. **Re-implement `define`'s `RunE` in bashy** (rebuild the store with the
   exported pieces, call `AddSkills`, then render). Rendering means a second
   copy of the unexported `writeConcept` — a second runner of the kind this
   repo's `wireLexicon` comment exists to forbid, drifting silently the first
   time lexicon changes its output.
2. **Edit `../coreutils`** — out of this workspace's contract.

So today `bashy define conductor` still answers "unknown here" while
`bashy inspect actions --kind skill` lists it with a full facet. That gap is
the blocker; everything else in the story is delivered.

## The fix (verified, 24 lines in coreutils + 1 line in bashy)

**coreutils** — a `SkillSource` seam on pkg/lexicon, the same shape as
`SeatSource`, consulted by `buildFull`. Verified in a throwaway worktree of
coreutils `6fd41652`: `go test ./pkg/lexicon/` → `ok`, including the new
`TestDefine_SkillSourceSeam` (wired → the facet line; unwired → "unknown here").

```diff
diff --git a/pkg/lexicon/cli.go b/pkg/lexicon/cli.go
index 66b0883a..8f4abbb5 100644
--- a/pkg/lexicon/cli.go
+++ b/pkg/lexicon/cli.go
@@ -307,6 +307,14 @@ func writeConcept(out io.Writer, c *Concept) {
 // by any project rather than hard-wiring bashy.
 var KnownCommands []string
 
+// SkillSource is the seam to the skill catalog, injected by the embedding
+// shell — the same shape as SeatSource, and for the same reason: the ring is
+// mounted by the shell (its embedded FS is bashy's, not this package's), and
+// pkg/lexicon must not import pkg/skills (see SkillRow). nil means the host
+// cannot answer for skills, so `define <skill>` says "unknown here" rather
+// than guessing. WIRING IT IS THE LOAD-BEARING STEP.
+var SkillSource func() []SkillRow
+
 func build(opts []fleet.Option) *Store {
 	host, _ := os.Hostname()
 	return Build(fleet.New(opts...), Synopses, host, Overlay{})
@@ -348,6 +356,11 @@ func buildFull(opts []fleet.Option) *Store {
 	// missing: `define <this machine>` answered "unknown here", and `define
 	// steward` returned the verb rather than the seat somebody was holding.
 	s.AddReachable(Overlay{})
+
+	// The skill catalog — a capability, run by name, with its action facet.
+	if SkillSource != nil {
+		s.AddSkills(SkillSource(), Overlay{})
+	}
 	return s
 }
 
diff --git a/pkg/lexicon/define_test.go b/pkg/lexicon/define_test.go
index b3df7f8c..b479778d 100644
--- a/pkg/lexicon/define_test.go
+++ b/pkg/lexicon/define_test.go
@@ -1,6 +1,7 @@
 package lexicon
 
 import (
+	"bytes"
 	"os"
 	"slices"
 	"strings"
@@ -317,3 +318,33 @@ func TestDefineCmd_CredentialIsNotRenderedInEitherMode(t *testing.T) {
 		}
 	}
 }
+
+// A wired SkillSource makes `define <skill>` answer with the skill's facet;
+// unwired, the host says "unknown here" rather than guessing.
+func TestDefine_SkillSourceSeam(t *testing.T) {
+	defer func(prev func() []SkillRow) { SkillSource = prev }(SkillSource)
+	SkillSource = func() []SkillRow {
+		return []SkillRow{{Name: "go-health", Description: "build then test", FaceValid: true, Identity: "h" + strings.Repeat("0", 64), EffectCap: []string{"read"}}}
+	}
+	cmd := NewDefineCmd()
+	var out bytes.Buffer
+	cmd.SetOut(&out)
+	cmd.SetArgs([]string{"go-health"})
+	if err := cmd.Execute(); err != nil {
+		t.Fatal(err)
+	}
+	if !strings.Contains(out.String(), "runs: skill contract=dhnt latitude=exact authority=deterministic effects=read via dhnt (attest-jsonl)") {
+		t.Fatalf("define go-health did not print the skill facet:\n%s", out.String())
+	}
+	SkillSource = nil
+	out.Reset()
+	cmd = NewDefineCmd()
+	cmd.SetOut(&out)
+	cmd.SetArgs([]string{"go-health"})
+	if err := cmd.Execute(); err != nil {
+		t.Fatal(err)
+	}
+	if !strings.Contains(out.String(), "unknown here") {
+		t.Fatalf("unwired SkillSource still answered:\n%s", out.String())
+	}
+}
```

**bashy** — one assignment in `wireLexicon()` (`internal/agentos/agentos.go`),
which is the ONE place every lexicon entry point is configured:

```go
lexicon.SkillSource = lexiconSkillRows
```

plus a line in `TestLexiconEntryPointsWireTheFactStore` asserting
`lexicon.SkillSource != nil`, so an unwired seam fails the build instead of
reading as an empty catalog.

Verified end to end against the patched sibling through a scratch
`-modfile` (no sibling edited): with the seam wired, `bashy define conductor`
prints

```
runs: skill contract=dhnt latitude=judge authority=agentic effects=read,write,net,spend,time via dhnt (attest-jsonl)
```

Land the coreutils patch, bump `.sibling-pins` (owned by another worker this
sprint), then add the bashy line and its test assertion.
