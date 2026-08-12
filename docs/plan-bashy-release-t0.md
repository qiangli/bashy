# plan: `bashy release` T0 — wiring the embedded core

Companion to the plan of record, `../../docs/bashy-release-pipeline-design.md`.
That doc settles the *shape* (embed the permissive core, binmgr the tail) and
records the measurement behind it. This one settles the two decisions it left
open (§7.1 verb budget, §7.2 code home) and states what is built vs. what the
next slice wires.

## Decided: code home

`coreutils/pkg/release` — OSS, mounted by bashy via `internal/agentos`,
following the `pkg/{weave,craft,secrets}` precedent. Confirmed and built.

## Decided: verb budget — the `Why` field

`docs/orchestration-verb-consolidation-audit.md` requires a `Why` before a new
verb lands. `release`'s answer:

> **Distribution is not orchestration.** Every existing orchestration verb
> answers *who does the work and in what order* (`weave`, `sprint`, `dag`,
> `foreman`, `supervise`, `conductor`). `release` answers *what bytes leave
> this machine under what name* — it produces named, checksummed artifacts a
> third party consumes by name (cloudbox's `entity.OutpostRelease`, outpost's
> upgrade receiver, `binmgr`). No existing verb owns an artifact's identity,
> and folding it into `dag` would make the artifact contract invisible to the
> tool that has to honour it.

Two constraints ride along, and are the reason this is one verb and not a
family:

- **Stages are SUBCOMMANDS of `release`**, never top-level verbs. `release
  build|archive|checksum` run in-process; `release sign|sbom|publish-docker`
  dispatch to a binmgr-fetched, pinned binary. One verb, one UX, one entry in
  the atlas.
- **`release` is not a loop verb**, so it may declare the `net` effect (publish
  and fetch both need it) without tripping
  `pkg/atlas/localfirst_test.go`. `--snapshot` — the whole T0 slice — is
  local-first: no network, no credentials, no tag required.

## Built (coreutils)

`coreutils/pkg/release`, zero new modules (`go.mod`/`go.sum` untouched), hence
zero new license exposure:

| File | What |
|---|---|
| `config.go` | the `.goreleaser.yaml` subset, read with the yaml.v3 coreutils already links; unknown keys and unimplemented stages are refused **by name** |
| `template.go` | name-template rendering, `missingkey=error` |
| `plan.go` | build matrix → targets → archive plan; refuses name collisions |
| `archive.go` | byte-deterministic tar.gz / zip / binary |
| `run.go` | build → archive → checksum, `bashy-release-v1` artifact ledger |

Why original code rather than importing goreleaser's core: the three stages T0
needs live in `internal/pipe/*` and are **not importable at any price** (design
doc §1). Importing `pkg/{config,archive,context}` would buy the schema for
+6.7 MB and 39 modules and still leave every stage to write.

## Landed (bashy, this slice) — the wiring

The four touch points the plan named, plus their tests:

| Touch point | What landed |
|---|---|
| `internal/agentos/release.go` | the verb: `release --snapshot` / `release snapshot` / `release plan` / `release check`, flags `-C/--dir`, `-f/--config`, `--dist`, `--version`, `--skip-build`, `--json` |
| `internal/agentos/agentos.go` | `case "release"` in `Dispatch`, and `release` added to `alwaysShimVerbs` (so the bare name works inside the shell) |
| `internal/agentos/commands.go` | the one-line `verbSynopsis` entry |
| `internal/agentos/atlas.go` | the atlas entry, in `bashyOwnedVerbAtlas` |

End-user invocation:

```sh
bashy release --snapshot                     # in a project with .goreleaser.yaml
bashy release --snapshot --version 0.1.0     # name the build yourself
bashy release --snapshot --json              # the ledger on stdout (default under $BASHY_AGENTIC)
bashy release plan                           # what it would produce, without building
```

One run emits, into the config's `dist` (or `--dist DIR`): the archives, the
checksum manifest, and `release-ledger.json` — the `bashy-release-v1` ledger
naming every artifact with its sha256 and size, which is what a later stage
(smoke, publish, the fleet envelope) reads instead of re-deriving.

Three decisions this slice had to make that the plan left implicit:

1. **Atlas group is `toolchains`, not `build`.** The closed group vocabulary
   lives in `coreutils/pkg/atlas` and has no `build` member; this slice does not
   change coreutils. `toolchains` is where the build side of the surface already
   lives (`go`/`cmake`/`clang` — the toolchain `release` drives). Stage is
   `deploy`, tier is `workspace`, effects are exactly `read`/`write`/`exec` —
   **not** `net`, because `--snapshot` reaches no network. A test pins that.
2. **A snapshot's version is stated, never guessed.** With `--version` it is
   used verbatim (a leading `v` stripped). Without it, the config's
   `snapshot.version_template` renders over the resolved commit from a
   `0.0.0` base. Deriving a base from the newest tag would be a guess presented
   as a fact — and tag ordering is not lexical — so it stays Tier 1. Outside a
   git repo there is no commit to name the build after, and that is an error
   pointing at `--version`, not a build named after an empty string.
3. **Git is read pure-Go** (`coreutils/git`.`RevParse`, go-git), not by
   shelling out, so `bashy release` works on a node whose only toolchain is
   bashy. A dirty worktree is a stderr note, not a failure: the artifacts are
   legitimate, but the version names a commit they do not correspond to, so the
   run says so.

Tests: `internal/agentos/release_test.go` (artifacts + checksums + ledger
agreement, byte-determinism across two runs, stage refusal by name, the missing
config / no-version / `--dist` escape diagnostics, `plan` writing nothing, a
real `go build` end-to-end, and the catalog/atlas classification) and
`internal/agentos/release_e2e_test.go` (`-tags e2e`, the real binary). `release`
is also in the `native` set of `TestE2EAllListedCommandsDispatch`, so the
three-OS CI gate really invokes it.

The T0 exit criterion is now checkable end-to-end: a `.goreleaser.yaml`
expressing today's `release.yml` naming (`ycode-<os>-<arch>.tar.gz` +
`SHA256SUMS`) produces exactly those assets, and re-running produces the same
bytes. The plan-level half was already pinned by
`TestPlanReproducesYcodeArtifactNames`.

## Not in scope here

Tier 1 (`release smoke` over `dag --mesh`, `release publish`, `release
envelope`) and everything in design doc §6 (announce, blob storage, the
package-manager tail). Do not re-propose them.
