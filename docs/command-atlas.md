# The bashy Command Atlas

Status: design + shipped views (2026-07-08); default surface reorganized
by-how-it-runs + `graph`/`ast` collapsed to single subcommand-bearing verbs
(2026-07-10). Companion code: `coreutils/pkg/atlas` (the catalog),
`bashy/internal/agentos/{atlas,commands_sections}.go` (the merge + default
grouping), `bashy commands --view/--atlas/--idioms` (the views).

## 1. Why an atlas

Bashy's command surface (~340 unique names) reaches agents through one flat
catalog. **Since Sprint 167 (bashy 1.0.0) the default `bashy commands` is
core-first**: the first screen is the 35 commands an agent uses every turn in
seven rows (fleet · session · work · knowledge · comms · human · discovery),
then the visible extras, then one line of counts for the userland (bash
builtins · GNU coreutils · classic Unix · bin-managed externals) with the view
that expands it, then the count of hidden **experimental** commands. 325 names
in one wall was the problem; a first screen readable in one pass is the fix.
The underlying class taxonomy (`builtin`/`coreutils`/`verb`, §4) is unchanged,
and the partition now keys on the **origin** axis (§2.6) rather than on group
heuristics. That is the *classical* lens — what a command **is** and how it
runs. Agents planning work need more lenses:

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
| `origin` | **provenance lens (§2.6) — exclusive: `bash` \| `gnu` \| `unix` \| `external` \| `bashy`; mandatory** |
| `posix` | `true` for the 116 POSIX-required names (cross-cuts `origin`) |
| `core` | `true` for the bashy 1.0.0 core (35 commands, §2.6) |
| `os` | **platform lens (§2.7)** — the OSes the command is supported on (`darwin` \| `linux` \| `windows`); curated from the Windows/other stubs, cited in `platform.go` |
| `partial` | supported OSes where it runs with a documented gap (a flag/mode that errors "not supported on windows") |
| `portable` | `true` = full support on all three — the command a script can use as-is everywhere |
| `status` | `experimental` on a curated-hidden command that is unproven; `alias` on a curated-hidden name that is only a second spelling of a visible command (`podman`/`docker` → `oci`, `sphere` → `peer`); absent on a compatibility alias (§2.6) |
| `hidden` | `true` for the compatibility aliases and the curated experimental set (shown only with `--all`) |
| `alias_of` | `oci` for `sandbox`/`podman`/`docker`, `peer` for `sphere`, the singular for each plural; empty otherwise |

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
| `knowledge` | kb, skill (`skills` is its hidden plural alias) |
| `engines` | podman, docker, ollama, sphere |
| `forge` | git, git-scm, gh, loom |
| `toolchains` | go, cmake, clang, node, npm, npx, pnpm, yarn, python, pip, uv, mise, cargo, rustc, rustup, rust, java, javac, mvn |
| `storage` | rclone, zot, seaweedfs, kopia |
| `cluster-cloud` | kubectl, helm + every declarative-registry CLI (doctl today; aws/azure/gcloud when registered) |
| `platform` | commands, check, verify, self, run, secret, app, bootstrap, upgrade; `context` and `audit` remain platform rows as hidden aliases of `inspect`; `secrets`/`apps` are hidden plural aliases |
| `diagnostics` | **inspect** (bashy's self-inspection: `paths` · `mode` · `doctor` · `context` · `audit`), check, conform, gate, out, posix-gate, why; `doctor` remains a diagnostics row as a hidden alias of `inspect`. Rule: a new read-only self-view is a new `inspect` aspect, never a new verb |
| `account` | tessaro, login |

**Naming rule (2026-09-12): nouns are singular.** A verb that names a kind of thing is
`agent`, `model`, `tool`, `person`, `skill`, `secret`, `app`; enumeration is `list`, never an
`-s` suffix. The plurals (`agents models tools people skills secrets apps`), `messages`
(→ `mb`) and `issue` (→ `todo`) dispatch identically as **hidden aliases** (`alias_of`,
listed under `--all`). Exceptions are enumerated in `coreutils/pkg/atlas/naming_test.go`:
POSIX/bash/GNU names are frozen (`jobs`, `dirs`, `times`, `strings`, `users`, …) and
`commands` stays plural because `command` is a POSIX builtin and it is a subcommand-less
lister. Rationale in `naming-pass.md` §2026-09-12.

Notes: `foreman` is both an in-process tool and a front-door verb — one atlas
entry (group `orchestration`). `echo`/`false`/`pwd`/`true` exist in the
coreutils registry but resolve as builtins; in merged views they appear under
`shell`, and their coreutils atlas entries (group `shellutils`) serve
non-shell consumers (multicall, MCP). `ast` and `graph` (group `code-intel`)
are each a **single command with subcommands** (`ast symbols/search/refs/map/
query`; `graph build/stats/neighbors/impact/path/hotspots/query · note/link/
observe/forget/recall/notes/pitfalls`) — one atlas entry each, dispatched
in-process. Likewise the catalog nouns are one entry each with a shared CRUD
shape: `tool`/`model`/`agent` (`list/show [--field PATH]/add [--set PATH=VALUE]/
set [--set PATH=VALUE|--unset PATH]/rm/edit/schema [--json]/verify/sync` — `schema`
lists the dotted paths `--set`, `--unset` and `--field` accept; an unknown path
fails loudly and prints it) and `skill` (`list/probe/show [--yaml|--json]/add
<dir>|<file.yaml>|-|<name> --description/rm/set/edit/verify/run/learn/promote/
export [--yaml]`). Embedded entries are immutable; every write is copy-on-write
into the local ring, so the verbs are the only supported way to edit the
catalog — never the files under `~/.config/bashy/`.

`kb` is likewise one atlas row with a subcommand-bearing surface (the atlas
classifies executable command names, not Cobra paths). Its `knowledge` row
carries `json` and fronts the five knowledge stages: `kb context --for TASK
--rings repo,host --forms note,page --budget 700 --json` (assemble), `kb
search`/`show` with singular `--ring` and `--form` (retrieve), `kb observe`
(observe), `kb validate --from-gate` (verify), and `kb note add --candidate`
(persist). `backlinks`, flag-only `doctor`, and `transfer --from memex` are on
the same row. Agents use these verbs; they never edit a store's `pages/*.md` or
`graph.jsonl` directly.

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
| `cross` | serves every stage | the userland, dag, kb, skill, secret, doctor, git, … |

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
| `json` | has a structured-output mode (`--json` or native equivalent) | tools: browser, fetch, duration, tz, ntp, sntp, tokens, foreman, ast, graph; verbs: weave, sprint, dag, sdlc, schedule, skill, kb, chat, agent, web, run, commands, context, check (verified flags); kubectl (native `-o json`) |
| `dry-run` | participates in the bashy dry-run manifest (`docs/dryrun.md`) | rm (destroy kind); redirection truncation is shell-level |
| `destructive` | can irreversibly delete/overwrite user data | rm, dd, shred, truncate |
| `read-only` | never mutates the filesystem (conservative) | cat, cmp, comm, df, diff, du, grep, head, hexdump, ls, od, readlink, realpath, stat, strings, tac, tail, tokens, tree, wc, which + `ast` (all subcommands are structural reads) |
| `cached` | keeps a persistent on-disk cache | graph (`.agents/bashy/graph.json`); self (bin cache); every `self-provisioning` verb (binmgr cache) |
| `budget` | token-budget-aware output | tokens, ast map (`--budget`) |
| `needs-network` | requires network to function (beyond first provision) | fetch, browser, ntp, sntp; git, gh, rclone, ollama, sphere, kubectl, helm, secret, tessaro, login, registry CLIs |
| `needs-pairing` | requires a Tessaro-paired machine / cloudbox token | sphere, tessaro, login, secret |
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
| `cred` | reads or writes credentials / secrets | secret, gh, git, `env`/`printenv` (emit the whole env) |
| `priv` | changes privilege, ownership, or a security label | chmod, chown, chgrp, chcon, runcon, chroot, mknod |
| `remote` | executes on **another host** (crosses the machine boundary) | dag (mesh), sphere, mirror, rclone, kubectl, helm, doctl |
| `persist` | leaves something that **outlives the session** | crontab, at, batch, nohup, schedule, every daemon, self/upgrade |
| `spend` | incurs metered cost (paid inference, pooled compute, cloud) | chat, meet, supervise, weave, sphere, ollama |

Load-bearing classification notes:

- **`env`/`printenv` carry `cred`.** They emit the whole environment, secrets
  included — which is exactly why the context-redaction allowlist must cover
  them, not just `bashy inspect context --json`.
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

### 2.7 Platform lens — where it runs (Sprint 167)

Every `cmds/` package compiles on every GOOS, so build tags prove nothing;
the truth is in the `*_other.go` / `*_windows.go` stubs, and
`coreutils/pkg/atlas/platform.go` curates from them, citing each stub. Three
values, not two: **unsupported** (the whole command errors there — `chown`,
`mkfifo`, `nice` on Windows; `ps`, `chcon` anywhere but Linux; the pinned
POSIX providers and `ollama` on Windows), **partial** (runs, with a named
gap — `more` interactive mode, `find -ctime`, `xargs -p`, `stty`, `sync` on
Windows), and **portable** (full on all three: 265 of 306 today).

**The default listing is this host's.** A listing that names `mkfifo` on
Windows advertises a command that will fail, so `bashy commands`, `-v --json`
`sections` and every `--view` are filtered to `runtime.GOOS`, with a footer
naming what was dropped (`not on darwin (2), not listed: chcon ps`).
`--os any` lifts the filter (`--all` implies it, since "everything" means
everything); `--os windows` asks about another host; `--portable` keeps only
the cross-platform set and composes with any view. `bashy commands NAME` is
never filtered — on macOS `ps` answers `NOT supported on darwin (only:
linux)`. The default `--json` (`bashy-commands-v1`) is filtered the same way
but gains no key: its keys are the guarded contract.

Windows entries are the ones to believe rather than check from a mac
(`docs/windows-crossplatform-uniformity.md`): a change to `platform.go`
wants a run on a real Windows host, not a green cross-compile.

### 2.6 Origin lens — who defined it (Sprint 167)

The class split says how a name *resolves*; the origin says who *defined* it.
The bashy-added group has a name of its own — the **yoke commands** — so it can
be referred to as easily as the other four (decision 2026-09-13, Sprint 167).
`bashy commands` minus yoke is the **classic** surface. Every yoke command is
agentic in the sense that matters — built for agentic tools — and many are
deterministic rungs that need no model. **Admission rule (operator,
2026-09-13):** bashy is designed for agentic tools, so a newly introduced
command is yoke *because* it is meant for them; a command with no agentic
intent does not earn a place in the binary — write it in Bash++ instead.
The two disagree in useful ways: `printf` resolves as a bash builtin but is a
GNU coreutils program; `awk` is in-process Go but nobody at GNU wrote it;
`m4` is POSIX-required and exec'd from a pinned provider. One **exclusive**
origin per command, plus one cross-cutting tag:

| origin | label | meaning | count |
|---|---|---|---|
| `bash` | bash builtin | bash 5.3 builtin, contributed by the embedding shell (stamped in bashy: the atlas tables never see builtins) | 61 |
| `gnu` | GNU coreutils | GNU coreutils 9.x command reimplemented in Go (`atlas.GNUCoreutilsUpstream()`, 108 names, 3 unimplemented: chroot coreutils runcon) | 98 visible (105 in the tool table; 7 shadowed by builtins) |
| `unix` | classic Unix | other classic Unix tool reimplemented in Go — awk sed grep jq tar tree ed vi-less … | 48 |
| `external` | bin-managed external | binmgr CLI, toolchain provisioner, or pinned POSIX provider — exec'd, never linked (= `subclass` ∈ managed-external/provisioner, or a registry entry) | 45 |
| `bashy` | **yoke** (added by bashy) | the **yoke commands** — bashy's own agentic / yoke surface, its third substrate (Classic · Bash++ · Yoke). `commands` minus yoke = the classic surface. Every yoke command is built for agentic tools; *agentic* does not mean *needs a model* — the ladder has deterministic rungs (`tz clip duration tokens ntp`) that are yoke all the same. Wire value stays `bashy` (provenance = who); "yoke" is how the group is referred to, like "the GNU coreutils" | 53 visible + 22 experimental + 16 aliases |

`posix: true` = one of the 116 POSIX-required names (`atlas.PosixRequired()`,
ratcheted against `coreutils/docs/posix-required-commands.tsv`, the file
`posix-gate`'s spec is generated from). POSIX is deliberately **not** an origin:
it cuts across bash (`cd`), GNU (`cat`), classic Unix (`awk`) and external
(`m4`), so a flat enum would have to pick between "GNU" and "POSIX" for `cat`
and every reader would ask which won. (`sh` is the 116th name; it is a
Preamble shim, not a listed command, so the view shows 115.)

**The posix view.** `bashy commands --view posix` is the certification lens:
the 116 POSIX-required names in two labeled groups — **internal** (in the
bashy binary, pure Go, no fork: 20 shell builtins · 51 GNU · 34 classic Unix =
105) and **bin-managed** (exec'd from a locally built pinned upstream: 10, the
pure-Go debt) — 115 listed and an explicit `not listed` line for what the catalog cannot show
(`sh`, the Preamble's `--posix` shim). It is a filter as well as a view —
`--view posix --json` returns only those records with `filter: {posix: true}`
— so an agent can ask "which of the 116 does this build provide, and how"
in one call. `bashy posix-gate spec` remains the *certified* projection; this
view is the catalog's answer, and a name on its `not listed` line other than
`sh` is a gap.

**The external view.** `bashy commands --view external` is "what is still not
pure Go" as one command: the 45 bin-managed names by kind — pinned POSIX
providers (exactly `ar ctags ex localedef lp m4 man nm strip vi` — the
**pure-Go debt**; each one implemented in Go leaves the list), managed
externals (git gh act kubectl …, plus `posix-providers` and `why` — wrapped
tools with their own release train), and toolchain provisioners (go node python …).
`--json` returns only those records with `filter: {origin: external}`. This
view matters until the POSIX providers are all reimplemented; it is the
progress meter for that work.

Origin is stamped per entry in `coreutils/pkg/atlas/origin.go`
(`classifyOrigins`, after the subclass passes and before the alias pass so an
alias inherits it) and ratcheted in `origin_test.go`: every entry has one,
origin follows subclass, tool-table counts are pinned, aliases inherit.

**The 1.0.0 core and the experimental set.** The visible/hidden split is a
*maturity* claim, not a removal. `core: true` marks the 35 commands the
operator named as used and dogfooded — fleet (`agent model tool skill person
whois capability`) · session (`chat delegate foreman coach handoff resume
claim`) · work (`sprint todo dag weave gate`) · knowledge (`kb graph craft
secret`) · comms (`inbox mb meet ping notify bus activity`) · human (`app ask
browser fetch`) · discovery (`commands`). Eighteen more bashy-added commands
stay visible by name (`inspect oci sandbox ollama peer dks login tessaro release
transpile dhnt otel duration tz ntp sntp clip ast`, plus `agentic` — a
**Bash++ reserved word**, never a hide candidate). The
remaining 22 — `supervise judge pair sdlc schedule herald · define lexicon
search sota · check conform · run out full · podman docker · sphere · self web
· tokens posix-gate` — are **curated-hidden**: `status: "experimental"`,
`hidden: true`, out of the default listing and `--agentic`, back under
`--all`, answered by `commands NAME`, and **dispatched byte-identically with
their bare shims intact** (`curatedHiddenVerbs` in `agentos.go` is a separate
list from `hiddenFrontDoorVerbs`, which also strips the shim). A command
graduates by leaving that list with a gate in hand.

**The container engine has one canonical name and three spellings** (operator,
2026-09-13): `oci` is the **standard** name — the O3 pillar (ollama · oci ·
otel), the spec the engine implements — and the canonical atlas entry;
`sandbox` is the **popular** name (the tier-3 venue word, a visible alias);
`podman` and `docker` are **vendor** spellings (hidden aliases, kept for
callers). All four dispatch to the embedded podman engine. The sphere tier
follows the same rule: `peer` is the canonical (taught) name, `sphere` — the
tier word — its hidden alias.
`bashy commands podman` says `hidden spelling of \`bashy oci\`` — its
`status` is `alias`, not `experimental`, because it is hidden as a spelling,
not as unproven (the engine is on the first screen). The default listing's
"experimental" count therefore excludes the three spellings (19), which
`--all` lists under hidden aliases instead.

### 2.8 Refs — one address for everything bashy can name (Sprint 168)

An agent has to learn **two things**:

1. **Write `kind:id`.** In prose as `[[kind:id]]`, on the command line and in
   JSON as `kind:id`: `kb:deploy-runbook`, `todo:a5f5cfc8`, `sprint:168`,
   `run:coreutils-21`, `meet:<id>`, `mb:412`, `bus:77`, `agent:codex`,
   `person:<handle>`, `host:<name>`, `tool:codex`, `model:opus5`,
   `skill:conductor`, `episode:<id>`, `role:conductor:168`. Fifteen kinds,
   closed and ratcheted (`coreutils/pkg/ref`); a prefix outside the list is a
   word, not a ref — `codex:gpt5.6-sol` stays a `tool:model` binding.
2. **Ask `bashy define kind:id`.** It answers with the record's title, status,
   where it lives and the command that opens it (`--json` for the node).
   Exit 1 only for a real ref that names nothing — and it says which of three
   things happened: no such id, not a kind, or no resolver wired on this build.

`urn:dhnt:kind:id` is the same ref spelled for text that leaves bashy (a URL,
another tool's store, an OTel attribute); every parser accepts it, nothing
emits it by default. Every listing prints the ref it lists (`todo list`,
`kb list`, `sprint show`, `mb --history`, `weave list`, `whois`), so a row an
agent reads is a citation it can write. Each store resolves its own kind;
`todo show --links` / `sprint show --links` / `meet show --links` classify a
citation as resolved · external (another store's kind) · unknown (not a kind)
· dangling. There is deliberately no `mb show --links`: a citation inside a
post is `define`'s job. Gate: the umbrella's `script/e2e-refs.sh`.

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
bashy commands --view origin        # who defined each name: bash · gnu · unix · external · bashy (* = POSIX, ~ = experimental)
bashy commands --view posix         # the 116 POSIX-required utilities by who provides each one here; names the unlisted (sh); --json = the filtered records
bashy commands --view external      # bin-managed: pinned providers (POSIX ones = the pure-Go debt) · managed externals · toolchain provisioners; --json = the filtered records
bashy commands --view portable      # runs as-is on windows · macOS · linux, by origin; the rest listed with where they DO run / their documented gap
bashy commands --os windows         # platform filter (default: THIS host; `any` lifts it; `--all` implies any) — composes with every view
bashy commands --portable           # only full-support-everywhere commands — composes with every view (`--view posix --portable`)
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
- `--json` composes with every view; `--all` adds the hidden aliases
  (`"hidden":true`) and the curated experimental commands
  (`"hidden":true,"status":"experimental"`); agent mode (`$BASHY_AGENTIC`)
  defaults to JSON as today. Every record carries `origin` (+ `posix`, `core`).
- `bashy commands NAME --features` gains additive keys: `group`, `tier`,
  `caps`, `subclass`, `origin`, `posix`, `core`, `status`, and `use` (the
  taught name of a hidden engine) (legacy keys unchanged). The text form
  prints one provenance line: `origin: GNU coreutils · POSIX-required`.
- `-v --json` `sections` additively carries `core` (the seven rows), `more`,
  and — with `--all` — `experimental` and `aliases`, beside the v1
  `shell/coreutils/classic/external/diagnostics/agent` partition, which is now
  keyed on `origin`.
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

### 4.1 The action facet — `bashy inspect actions`

The atlas classifies a command; the **action facet** (`coreutils/pkg/lexicon/action.go`)
answers a different question for everything runnable — *what happens if I run
this, and how much freedom does running it take* — in one vocabulary across
**four families**: **command** (an atlas entry: exact, deterministic, executor
builtin/coreutils/verb, effects off the record), **script** (a family the facet
names but nothing projects yet — `inspect actions --kind script` says so and
`inspect context` reports it as `0`, never omits it), **agent** (a fleet
binding: judge/agentic by definition, identity `tool:model`, executor
`agentlaunch:<tool>`), and **skill** (a catalog skill: contract `dhnt` with the
face's content address and declared effect-cap when `skill.dhnt` is valid, else
`metadata-checks` from `check-*` bindings, else `none`; one judge step makes the
run agentic). `bashy inspect actions [--json] [--kind command|script|agent|skill]`
is an *aspect* of `inspect` (never a verb) that lists those facets — the
**generic half only** (`kind name identity contract latitude authority
effects_declared executor`; no path, host or Location, so two hosts' output
diffs into what one can run and the other cannot), sorted by kind then identity,
as a `bashy-inspect-v1` envelope. It is not a second projection: the rows are
the facets of the same lexicon store `bashy define NAME` answers from (the
`runs:` line, and the nested `action` object in `--json`), so the two cannot
disagree. `bashy inspect context --json` carries the per-family counts as
`actions: {command, script, agent, skill}`.

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
