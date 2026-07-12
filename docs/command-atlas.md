# The bashy Command Atlas

Status: design + shipped views (2026-07-08); default surface reorganized
by-how-it-runs + `graph`/`ast` collapsed to single subcommand-bearing verbs
(2026-07-10). Companion code: `coreutils/pkg/atlas` (the catalog),
`bashy/internal/agentos/{atlas,commands_sections}.go` (the merge + default
grouping), `bashy commands --view/--atlas/--idioms` (the views).

## 1. Why an atlas

Bashy's command surface (~250 unique names) reaches agents through one flat
catalog. `bashy commands` groups it by **how each command runs**: a *builtins*
umbrella (shell builtins · in-process GNU coreutils · in-process classic tools —
all zero-fork), the exec'd downloaded *externals*, and bashy's native *agent*
features by execution venue. The underlying class taxonomy
(`builtin`/`coreutils`/`verb`, §4) is unchanged — the default grouping derives
from it plus the GNU-coreutils set (coreutils vs classic) and Subclass/Tier. That
is the *classical* lens — what a command **is** and how it runs. Agents planning
work need more lenses:

- **where it runs** — the execution tier (userland / workspace / sandbox /
  sphere / cluster / cloud / account, per `dhnt` execution-tiers vocabulary);
- **what it can do for an agent** — structured output, dry-run participation,
  destructiveness, network/pairing requirements, self-provisioning,
  token-budgeted output;
- **what it is used with** — recurring composites (`find | xargs`,
  `git`+`gh`+`act`, the `weave`/`sprint`/`foreman`/`dag` suite).

The atlas is one curated, test-ratcheted catalog carrying all of these axes,
rendered as views in `bashy commands` and importable as a Go package so other
subsystems — **`bashy dag` target preflight in particular** — can consult the
same data programmatically.

Design rules:

1. **Curated, never inferred.** Every group/tier/cap assignment is a hand-set
   table entry, kept honest by coverage tests (a new tool that lacks an atlas
   entry fails the build). No flag-scraping heuristics.
2. **Schema-stable, presentation-free.** The default `bashy commands` **`--json`**
   (`bashy-commands-v1`) stays byte-identical — its keys (`builtins`/`coreutils`/
   `verbs`) are a guarded contract. The default **human text** is the
   by-how-it-runs surface (§1); it is a rendering over the same records, not a
   schema, so it may be reorganized without a version bump (`-v --json` adds a
   `sections` object mirroring it). Atlas views carry their own schema id
   (`bashy-atlas-v1`).
3. **Closed vocabularies.** Groups, tiers, and caps are fixed lists; an
   unknown value in a filter is an exit-2 error that prints the vocabulary,
   and an unknown value in the tables is a test failure.

## 2. The axes

Each command has one record:

| field | meaning |
|---|---|
| `name` | command name |
| `synopsis` | one line (from `tool.Synopsis` / `verbSynopsis`; builtins have none) |
| `class` | classical: `builtin` \| `coreutils` \| `verb` (unchanged v1 taxonomy) |
| `subclass` | refines `verb` only: `provisioner` \| `managed-external` \| `""` |
| `group` | functional lens, one group per command (§2.1) |
| `tier` | execution-tier lens (§2.2) |
| `sdlc` | **SDLC-stage lens (§2.2a) — mandatory for every verb; `addVerb` panics without one** |
| `resolver` | existing: `bash-builtin` \| `bashy-in-process` \| `bashy-front-door` \| `managed-container-or-system` |
| `caps` | agentic capability flags (§2.3) |
| `effects` | security/privacy/governance effects (§2.5) — **mandatory, ≥1 per command** |
| `hidden` | `true` for `bootstrap`/`upgrade` (shown only with `--all`) |
| `alias_of` | `podman` for `docker`; empty otherwise |

### 2.1 Group vocabulary

One group per command. The classical coreutils three-way split (fileutils /
textutils / shellutils) is kept for the userland; the former "misc/extended"
bucket is split into honest functional groups.

| group | members |
|---|---|
| `shell` | the shell builtins (from `interp.BuiltinNames()`), including the coreutils names they shadow (`echo`, `false`, `pwd`, `true` resolve as builtins) |
| `fileutils` | basename, chcon, chgrp, chmod, chown, clip, cp, dd, df, dir, dircolors, dirname, du, find, install, link, ln, ls, mkdir, mkfifo, mknod, mktemp, mv, readlink, realpath, rm, rmdir, shred, stat, sync, tar, touch, tree, truncate, unlink, vdir |
| `textutils` | awk, b2sum, base32, base64, basenc, cat, cksum, cmp, comm, csplit, cut, diff, expand, fmt, fold, grep, gunzip, gzip, head, hexdump, join, jq, md5sum, more, nl, numfmt, od, paste, pr, ptx, sed, sha1sum, sha224sum, sha256sum, sha384sum, sha512sum, shuf, sort, split, strings, sum, tac, tail, tee, tokens, tr, tsort, unexpand, uniq, wc, xargs, zcat |
| `shellutils` | arch, at, atq, atrm, batch, cal, chroot, crontab, date, duration, echo, env, expr, factor, false, groups, hostid, hostname, id, logname, ncal, nice, nohup, nproc, ntp, pathchk, pinky, printenv, pwd, runcon, seq, sleep, sntp, stdbuf, stty, time, timeout, true, tty, tz, uname, uptime, users, watch, which, who, whoami, yes |
| `code-intel` | ast, graph |
| `net` | browser, fetch, web, curl |
| `orchestration` | weave, sprint, dag, foreman, sdlc, chat, meet, agent, schedule, act, act-runner, mirror |
| `knowledge` | kb, skills |
| `engines` | podman, docker, ollama, sphere |
| `forge` | git, git-scm, gh, loom |
| `toolchains` | go, cmake, clang, node, npm, npx, pnpm, yarn, python, pip, uv, mise, cargo, rustc, rustup, rust, java, javac, mvn |
| `storage` | rclone, zot, seaweedfs, kopia |
| `cluster-cloud` | kubectl, helm + every declarative-registry CLI (doctl today; aws/azure/gcloud when registered) |
| `platform` | commands, context, doctor, check, verify, self, run, secrets, bootstrap, upgrade |
| `account` | tessaro, login |

Notes: `foreman` is both an in-process tool and a front-door verb — one atlas
entry (group `orchestration`). `echo`/`false`/`pwd`/`true` exist in the
coreutils registry but resolve as builtins; in merged views they appear under
`shell`, and their coreutils atlas entries (group `shellutils`) serve
non-shell consumers (multicall, MCP). `ast` and `graph` (group `code-intel`)
are each a **single command with subcommands** (`ast symbols/search/refs/map/
query`; `graph build/stats/neighbors/impact/path/hotspots/query · note/link/
observe/forget/recall/notes/pitfalls`) — one atlas entry each, dispatched
in-process.

The **default `bashy commands` surface** groups the userland further than
these groups: `fileutils`/`textutils`/`shellutils` tools split into
*coreutils* (name ∈ the canonical GNU coreutils set — the same list behind
`--gnu`) vs *classic* (jq/awk/sed/grep/find/tree/… — bundled but not GNU
coreutils), both under the in-process *builtins* umbrella; the `code-intel`/
`orchestration`/`knowledge`/`net` in-process tools render under *agent/ext*
with the front-door verbs. This is a presentation split derived from group +
GNU membership, not a new atlas axis.

### 2.2 Tier lens

Vocabulary is locked by `dhnt/docs/execution-tiers.md`:
`userland` · `workspace` · `sandbox` · `sphere` · `cluster` · `cloud`, plus
`account` (the front door beside the stack). Naming discipline applies:
sandbox = OCI/podman only; sphere ≠ cluster; cloud (hosted providers) ≠
cluster (your own DKS).

| tier | commands |
|---|---|
| `userland` | everything not listed below — the whole coreutils userland, code-intel, kb/skills, chat/meet/agent/schedule/mirror, git/gh, storage, toolchains, platform, net (default tier) |
| `workspace` | weave, sprint, dag, loom, sdlc |
| `sandbox` | podman, docker, act, act-runner |
| `sphere` | sphere, ollama |
| `cluster` | kubectl, helm |
| `cloud` | registry CLIs with `Entry.Tier == 6` (doctl, future aws/azure/gcloud) — derived from the registry, never hand-listed here |
| `account` | tessaro, login |

The tier means "the tier this command operates/fronts", not "where the binary
runs" (every binary runs in userland). `foreman` stays `userland`: it manages
a session on this node; the workspaces it drives are weave's.

### 2.2a SDLC lens — the spine

`plan → code → test → deploy`, plus `cross` for the commands that serve every
stage (the userland, knowledge, identity, diagnostics). One stage per command.

| stage | meaning | commands |
|---|---|---|
| `plan` | decide what to build | sprint, meet |
| `code` | build it | weave, chat, foreman, agent, the toolchains (go/cargo/npm/…) |
| `test` | decide pass/fail | check, verify, act, act-runner |
| `deploy` | ship it | sdlc, kubectl, helm, sphere, tier-4+ registry CLIs |
| `cross` | serves every stage | the userland, dag, kb, skills, secrets, doctor, git, … |

**This axis exists to ask one question of every new verb: *which stage do you
serve that nothing else already does?*** It is not decoration. bashy's agentic
surface grew piecemeal until the **Code stage carried six overlapping verbs**
(weave, supervise, foreman, chat, fanout, `sdlc delegate`) while the **Test
stage carried none** — the gate was spelled four incompatible ways across four
packages. Nobody could see that, because there was no axis on which to see it.

Two rules keep it honest:

1. **A stage is mandatory for a verb, enforced at init.** `atlas.addVerb` panics
   without one, so a verb that cannot answer the question cannot start the
   binary. This is deliberately harsher than a coverage test, because a test can
   be defaulted around — and this one was. `bashy`'s `verbAtlasRecord` used to
   invent `GroupPlatform`/`TierUserland` for any verb missing an entry; those are
   *valid* values, so the coverage test passed and the omission was invisible.
   **`fanout` shipped that way** — zero callers, zero skills, no atlas entry — and
   the ratchet meant to catch it was the very thing being defeated. It was
   deleted 2026-07-12 when the axis landed and it had no answer to give.
2. **Do not pave over a hole to make the table look tidy.** `dag` is `cross`, not
   `test`: it is a make-replacement that runs build, test *and* deploy targets.
   Filing it under `test` would make the Test stage look populated while the gate
   remained missing. The thin `test` column is the finding, not a defect in the
   axis — `bashy gate` is what fills it.

View it with `bashy commands --view sdlc`.

### 2.3 Capability vocabulary

Hand-curated; every flag is defensible today with the evidence below. A cap
is omitted when unsure (absence is *unknown*, not *no*).

| cap | meaning | seeded on (evidence) |
|---|---|---|
| `json` | has a structured-output mode (`--json` or native equivalent) | tools: browser, fetch, duration, tz, ntp, sntp, tokens, foreman, ast, graph; verbs: weave, sprint, dag, sdlc, schedule, skills, kb, chat, agent, web, run, commands, context, check (verified flags); kubectl (native `-o json`) |
| `dry-run` | participates in the bashy dry-run manifest (`docs/dryrun.md`) | rm (destroy kind); redirection truncation is shell-level |
| `destructive` | can irreversibly delete/overwrite user data | rm, dd, shred, truncate |
| `read-only` | never mutates the filesystem (conservative) | cat, cmp, comm, df, diff, du, grep, head, hexdump, ls, od, readlink, realpath, stat, strings, tac, tail, tokens, tree, wc, which + `ast` (all subcommands are structural reads) |
| `cached` | keeps a persistent on-disk cache | graph (`.agents/bashy/graph.json`); self (bin cache); every `self-provisioning` verb (binmgr cache) |
| `budget` | token-budget-aware output | tokens, ast map (`--budget`) |
| `needs-network` | requires network to function (beyond first provision) | fetch, browser, ntp, sntp; git, gh, rclone, ollama, sphere, kubectl, helm, secrets, tessaro, login, registry CLIs |
| `needs-pairing` | requires a Tessaro-paired machine / cloudbox token | sphere, tessaro, login, secrets |
| `self-provisioning` | download → verify → cache → exec on first use | all toolchain provisioners, git/git-scm/gh/curl, rclone, loom, zot, seaweedfs, kopia, act, act-runner, kubectl, helm, mise, uv, registry CLIs |
| `spawns-processes` | executes external processes (documented command-wrapper exception or managed external) | xargs, timeout, time, watch, nice, nohup, chroot, runcon, stdbuf, at, batch; run, chat, meet; every managed external / provisioner |
| `daemon` | starts or manages a long-running service | ollama, loom, zot, seaweedfs, kopia, act-runner, mirror, podman, docker, foreman |

Deferred (roadmap, not shipped): `deterministic` — claiming byte-stable
GNU-conformant output needs the fidelity harness
(`dhnt/docs/coreutils-fidelity-perf-harness-spec.md`) as recorded evidence,
exactly like the empty `gnuCoreutilsFullyConformant` list.

### 2.5 Security-effect vocabulary

Where `caps` describe what a command is *for*, `effects` describe what it can
*do* to the machine, the data, or the outside world — the security / privacy /
governance lens. This axis differs from `caps` in one load-bearing way: it is
**mandatory and fail-closed**. Every entry declares at least one effect, the
coverage ratchet fails the build on any command with none, and `EffPure` is the
explicit "considered, benign" declaration — so a new command can never slip in
unclassified. `caps` omit-when-unsure; `effects` must decide.

The first six atoms mirror the dhnt skill-CNL effect lattice
(`coreutils/pkg/skills` → `github.com/dhnt/dhnt/skills`); the last five are the
finer distinctions a shell an agent drives needs. A future policy engine
projects the 11 onto the dhnt 6 for skill-cap compatibility.

| effect | meaning | examples |
|---|---|---|
| `pure` | deterministic, no governed side effect (exclusive — never combined) | true, false, echo, seq, expr, basename |
| `read` | reads filesystem / host state / input data (the privacy surface) | cat, ls, grep, stat, env, printenv |
| `write` | mutates the filesystem or host state | cp, mv, sed, tee, mkdir, graph |
| `destroy` | can **irreversibly** lose data | rm, dd, shred, truncate, unlink |
| `net` | opens a network connection (egress / exfiltration surface) | fetch, browser, git, curl, kubectl |
| `exec` | spawns a process bashy no longer governs | xargs, find, awk, env, chroot, all agent-spawning verbs |
| `cred` | reads or writes credentials / secrets | secrets, gh, git, `env`/`printenv` (emit the whole env) |
| `priv` | changes privilege, ownership, or a security label | chmod, chown, chgrp, chcon, runcon, chroot, mknod |
| `remote` | executes on **another host** (crosses the machine boundary) | dag (mesh), sphere, mirror, rclone, kubectl, helm, doctl |
| `persist` | leaves something that **outlives the session** | crontab, at, batch, nohup, schedule, every daemon, self/upgrade |
| `spend` | incurs metered cost (paid inference, pooled compute, cloud) | chat, meet, supervise, weave, sphere, ollama |

Load-bearing classification notes:

- **`env`/`printenv` carry `cred`.** They emit the whole environment, secrets
  included — which is exactly why the context-redaction allowlist must cover
  them, not just `bashy context --json`.
- **`exec` marks the governance boundary.** Once a command spawns an external
  process, the pure-Go userland, the advisor, and the audit hook do not reach
  across the `execve`. The agent-orchestration verbs (weave/chat/meet/…) and the
  toolchain provisioners (npm/pip/… run install scripts) are all `exec`.
- **`dag` is `remote`.** A `Host:`-tagged target body is piped to a remote
  `bash -s` (`coreutils/pkg/dag/exec_mesh.go`) — arbitrary remote execution
  driven by a markdown file. It was previously tagged only `json`.
- Registry CLIs derive effects from tier: a tier-4+ provider CLI (doctl) is
  `remote`; a tier-2 local tool (ripgrep) is `exec`+`net` but **not** remote.

### 2.4 Idioms — the composite lens

Idioms are a **separate top-level curated list**, not a per-command
`pairs_with` field: they are n-ary and cross-class, and one source of truth
beats N duplicated fragments. Record:
`{id, commands[], pattern, note, fused?, tier}`.

Seed set:

| id | pattern | note |
|---|---|---|
| `count-matches` | `grep PAT F \| wc -l` | fused: `grep -c PAT F` — one process, one pipe fewer |
| `top-n` | `… \| sort \| uniq -c \| sort -rn \| head` | fusion candidate (bounded-heap top-N verb); no fused form shipped yet |
| `find-exec` | `find … -print0 \| xargs -0 CMD` | the canonical scale-out; prefer `-print0/-0` for arbitrary names |
| `scoped-cd` | `(cd DIR && CMD)` | subshell keeps the cwd change scoped; agents should avoid bare `cd` |
| `list-inspect` | `ls` → `stat FILE` | enumerate, then inspect the interesting entry precisely |
| `tempfile-cleanup` | `t=$(mktemp) && trap 'rm -f "$t"' EXIT` | leak-free scratch files |
| `archive` | `tar -czf out.tgz DIR` | tar+gzip in one call; avoid `tar \| gzip` |
| `fetch-extract` | `fetch --json URL \| jq .field` | HTTP + structured extraction without a browser |
| `forge-loop` | `git` + `gh` + `act` | commit/push → PR → run the workflow locally before CI |
| `fleet-suite` | `weave` + `sprint` + `foreman` + `dag` | the orchestration suite: plan (sprint) → isolate/run (weave) → steer (foreman) → deterministic targets (dag) |
| `cluster-deploy` | `kubectl` + `helm` | inspect the cluster, install/upgrade via charts |
| `pair-first` | `login` before `sphere`/`kubectl` | tiers 4–5 need a Tessaro-paired machine |

Growth rule: adding an idiom edits this doc **and** the table in
`pkg/atlas`; the test asserts every referenced command exists in the catalog.

## 3. Data home

**`coreutils/pkg/atlas`** — stdlib-only, no deps — holds the whole catalog:
tool entries, verb entries, vocabularies, idioms, and accessors
(`Lookup`, `Tools`, `Verbs`, `Idioms`, `Groups`, `Tiers`, `Capabilities`).

Why coreutils, not bashy-internal: the atlas is an **execution-assist
substrate**, not just presentation. `pkg/dag` (target preflight), the
advisor, `mcp/` (list_tools), and the multicall binary must be able to
import it; `bashy/internal/agentos` would wall it off. Bashy contributes only
what it alone knows: the builtin name set (`interp.BuiltinNames()`), shim
visibility (agent-mode provisioners appear only in agent mode), the `docker`
alias, and registry-derived tiers (`registry.Entry.Tier`, int → name).

Rejected alternatives: (a) tables inside `commands.go` — invisible to
dag/MCP/multicall and drift-prone; (b) extending `tool.Tool` at registration
— touches 150+ cmd packages and pollutes a deliberately minimal invocation
contract with catalog metadata.

**Drift control (the core discipline, mechanized):**

- coreutils: `pkg/atlas` coverage test (external test package blank-importing
  `cmds/all`, `cmds/graph`, `cmds/foreman`) asserts the atlas tool table ==
  `tool.Names()` **exactly** — a missing or stale entry fails; vocabularies
  are closed; idioms reference only known names.
- bashy: `TestAtlasCoversEveryCommand` asserts every builtin/tool/verb the
  live catalog reports (including hidden verbs and agent-mode provisioners)
  resolves to a group + tier.
- Adding a command = register the tool + add one atlas line; the tests tell
  you if you forget either.

## 4. The views

Default `bashy commands` **`--json`** stays schema-stable
(`bashy-commands-v1`; `-v --json` additively carries a `sections` object of the
by-how-it-runs grouping); the default human text is that same five-section
surface (§1). Atlas views emit `bashy-atlas-v1`.

```
bashy commands --view tier          # grouped by execution tier, counts per tier
bashy commands --view group         # the functional-group lens
bashy commands --view capabilities  # per-cap command lists
bashy commands --view effects       # per-security-effect command lists (§2.5)
bashy commands --view classic       # explicit alias for the default output
bashy commands --tier workspace     # filter to one tier (implies tier view)
bashy commands --group code-intel   # filter to one group
bashy commands --cap json           # filter to one capability
bashy commands --effect destroy     # filter to one security effect (cred/priv/remote/…)
bashy commands --idioms             # the curated composite/idiom list
bashy commands --atlas              # full per-command records (the machine surface)
```

- Flags accept `--flag value` and `--flag=value`.
- Unknown tier/group/cap/effect → exit 2, with the closed vocabulary printed so
  an agent can self-correct in one round trip.
- `--json` composes with every view; `--all` adds hidden verbs
  (`"hidden":true`); agent mode (`$BASHY_AGENTIC`) defaults to JSON as today.
- `bashy commands NAME --features` gains additive keys: `group`, `tier`,
  `caps`, `subclass` (legacy keys unchanged).
- MCP `list_tools` `ToolInfo` gains additive `group` + `caps` fields.
  Multicall `--list` output stays byte-identical (richer listing = roadmap).

`--atlas --json` shape:

```json
{"schema_version": "bashy-atlas-v1",
 "tiers": ["userland","workspace","sandbox","sphere","cluster","cloud","account"],
 "groups": ["shell","fileutils","textutils","shellutils","code-intel","net",
            "orchestration","knowledge","engines","forge","toolchains",
            "storage","cluster-cloud","platform","account"],
 "capabilities": ["json","dry-run","destructive","read-only","cached","budget",
                  "needs-network","needs-pairing","self-provisioning",
                  "spawns-processes","daemon"],
 "security_effects": ["cred","destroy","exec","net","persist","priv","pure",
                      "read","remote","spend","write"],
 "commands": [
   {"name":"grep","class":"coreutils","group":"textutils","tier":"userland",
    "resolver":"bashy-in-process","caps":["read-only"],"effects":["read"],"synopsis":"…"},
   {"name":"weave","class":"verb","group":"orchestration","tier":"workspace",
    "resolver":"bashy-front-door","caps":["json"],"synopsis":"…"},
   {"name":"docker","class":"verb","group":"engines","tier":"sandbox",
    "resolver":"bashy-front-door","alias_of":"podman","caps":["daemon","spawns-processes"]}],
 "idioms": [
   {"id":"count-matches","commands":["grep","wc"],"pattern":"grep PAT F | wc -l",
    "fused":"grep -c PAT F","note":"one process, one pipe fewer","tier":"userland"}]}
```

## 5. The dag lens — the atlas as an execution-assist substrate

`pkg/atlas`'s Go API is shaped for `bashy dag` from day one; the features
below are the roadmap (not built yet):

- **Target preflight** — dag (or `bashy check` over a `dag.md`) scans a
  target's script for command names and resolves each against the atlas:
  unresolvable names, `self-provisioning` commands ("first run downloads the
  Go toolchain"), `needs-network`/`needs-pairing` prerequisites, and
  `destructive` commands (gate behind confirmation / `--yes`).
- **Dry-run** — per-target dry-run manifests reusing the existing
  `dryrun.go` machinery and its JSON-lines kinds.
- **Placement / when-conditions** — a target whose commands carry tier
  `cluster`/`sphere` implies reachability requirements; feed dag's
  when-conditions and `exec_mesh` placement decisions.
- **Cache validity** — `read-only` (and the future `deterministic`) caps
  inform whether a target's outputs are safely cacheable.

## 6. Per-tier agentic-extension roadmap

Proposals, explicitly not commitments; each stays honest about what exists.

- **workspace** — promote the conductor skill's deterministic steps to verbs
  **under the existing trees** (locked decision — no new top-level
  `conduct`): `bashy weave qualify` (the fleet-interview gauntlet as one
  verb), `bashy weave gate` (build the three-clause `--verify`),
  `bashy weave converge` (sequential gated merge), `bashy sprint judge`
  (judge-mode verdict for non-exit-coded goals) — beside the existing
  `weave conduct` (directive poller), `weave autopilot`, `weave baton`.
- **sandbox** — `bashy podman ps --json` parity audit (podman speaks
  `--format json` natively — verify pass-through fidelity); a capacity/
  rootless-status probe shaped like `doctor`; a warm-pool front-door (the
  engine's `pool.go` exists) is an open question — it implies a daemon
  lifetime, against bashy's no-daemon default character.
- **sphere** — beyond `sphere peers`/`status`: model-routing hints (which
  peer serves which model) and a capacity summary, JSON-first since the
  consumer is an agent; data comes from the outpost mesh agent.
- **cluster** — kubectl/helm already speak JSON natively; no wrapper. A DKS
  bundle-catalog verb is flagged needs-owner (catalog is a DKS-side concern).
- **cloud** — the declarative registry **is** the extension mechanism: new
  providers are data (`registry.Entry`), and the atlas derives tier/group
  automatically. Roadmap = aws/azure/gcloud entries, plus optional `Caps`
  on `registry.Entry` if provider CLIs diverge.

## 7. Measurement campaigns — proving the atlas pays for itself

Atlas features are refined through **bashy performance campaigns/sprints**
(conductor-driven, per the fleet playbook) and must be **measurably
beneficial to agentic tools** before graduating from proposal to commitment.
The yardstick is the north-star metric from
`dhnt/docs/bashy-agentic-performance-strategy.md`: **agentic-task cost =
wall-time × call-count × tokens**, measured with the instruments already
specced:

- **Corpus** — the benchmark suites mined in
  `dhnt/docs/coreutils-command-analysis.md` §2 (NL2Bash, InterCode, Koala,
  Terminal-Bench, ShellBench) plus live `tool.Names()`-frequency data from
  agent transcripts; leaderboard/authoritative sources are re-scanned each
  campaign so the corpus tracks what agents actually run.
- **Harness** — `coreutils/cmds/perfbench` and the bench container from
  `dhnt/docs/coreutils-fidelity-perf-harness-spec.md`; baselines committed
  under `results/`.
- **What gets measured per atlas feature**:
  - *idioms/fused verbs* — calls and tokens saved per fused form vs the
    pipeline it replaces, weighted by corpus frequency (the §4.3 fusion
    ranking is the seed);
  - *caps-driven preflight* (the dag lens) — failed-round-trips avoided
    (unresolvable names, missing network/pairing caught before execution);
  - *views* — token cost of discovery: `--view`/`--atlas` output size vs the
    prose docs an agent would otherwise read; budget-bounded output where the
    corpus shows discovery in hot paths.
- **Discipline** — a feature that doesn't move the metric on the corpus is
  dropped or re-scoped; claims name their corpus (never a bare "faster for
  agents"), mirroring the conformance rule that an empty certified list beats
  an unmeasured claim.

## 8. Verification & maintenance

- `cd coreutils && go test ./pkg/atlas/...` — exact-set coverage vs
  `tool.Names()`, closed vocabularies, idiom references.
- `cd bashy && go test ./internal/agentos/...` — merged-catalog coverage,
  per-view rendering, v1 byte-compat guard.
- E2E: `bashy commands --view tier --json | jq '.schema_version'` →
  `bashy-atlas-v1`; `bashy commands --tier cloud` → exactly the registry
  cloud entries; bogus vocabulary values → exit 2 + the vocabulary.
- **How to add a command**: register the tool (or shim the verb), add one
  entry to the `pkg/atlas` table (bashy-side table for bashy-only verbs);
  run the tests — they name anything you forgot. The snapshot doc
  `coreutils/docs/bashy-command-groups.md` is superseded as the live source;
  regenerate counts from `bashy commands --atlas --json` when citing them.
