---
id: f06ca924557c
kind: refactor
title: conform is the judge only; bashy's own test batteries move to selftest
seq: 3324
status: todo
priority: p1
labels:
    - conform
created: 2026-11-26T10:56:23.010154Z
sprint: 725
sprint_id: 5396d983-6e99-5b2b-8985-d08dc89fdef5
sprint_title: conform is the judge only; bashy's own batteries move to selftest
---

Operator ruling 2026-11-26 (from bashy todo 3257b40fcc9a, found in umbrella Sprint 723). bashy is for agents: one name must mean one thing.

## Why
`bashy conform` carries two unrelated things: bashy's own test batteries (compat, conformance, compliance, benchmark) and the dhnt/conform judge (run, cert, mutate, version). An agent told to run `conform run CONTRACT.md` must get the judge in any shell, with the same command line as the standalone `conform` binary.

## Code inspected (bashy main, 2026-11-26)
- `internal/agentos/verify.go`: `verifyCmd()` builds one cobra command, `Use: "conform [suite|run|cert|mutate|version]"`, adds the four suite subcommands from `suiteRegistry()` and four pass-through subcommands (`conformSuiteSubcommand`); `conformSchemaVersion = "bashy-conform-v1"`, envelope `Verb: "conform"`.
- `internal/agentos/agentos.go`: `case "conform", "verify":` dispatch (about line 1255), the verb list (about line 113) and the visible-verbs list (about line 208).
- `internal/agentos/cobra_schema.go`: `case "conform", "verify": return verifyCmd()`.
- `internal/agentos/commands.go`: the one-line description of `conform`.
- Tests: `verify_test.go`, `verify_conform_test.go`, `commands_test.go`, `release_bar_envelopes_test.go`.
- Docs: `docs/release-bar.md`, `docs/release-bar-v1.generated.md` (generated; regenerate, do not hand-edit).

## Change
1. New verb `selftest` = the batteries: `bashy selftest [compat|conformance|compliance|benchmark] [flags]`, `selftest --list [--json]`. The `--list --json` envelope keeps `schema_version: "bashy-conform-v1"` and its fields; only `verb` becomes `selftest`. `verify` stays an alias of `selftest`.
2. `conform` = the judge only. Every argument after `conform` goes verbatim to the dhnt/conform command line in-process (its `cli.Run`), so `bashy conform X` equals standalone `conform X` for every X, including `--help`, `version`, and subcommands added later. No per-subcommand list in bashy.
3. Compatibility for one release: `conform compat|conformance|compliance|benchmark ...` and `conform --list ...` forward to `selftest` with the same arguments and exit code, and print one line on stderr: `conform: the test batteries moved to "bashy selftest"; this forward will be removed`. Bare `bashy conform` prints the judge's usage.
4. Move the dhnt/conform requirement in go.mod to conform main at commit 2948278 or later (it adds `conform spec check`), with go.sum. Nothing else in go.mod changes.
5. Update the verb lists, the one-line descriptions (two entries: `conform`, `selftest`), help text, and the two release-bar docs (regenerate the generated one with its existing generator).

## Acceptance
- `bashy conform version` prints the dhnt/conform version; `bashy conform spec check DIR` runs (exit 0 on a clean spec folder such as examples/desk/form); `bashy conform run --help` prints the judge's run usage.
- `bashy selftest --list --json` prints the same envelope as `conform --list --json` did before, with `verb: "selftest"`.
- `bashy conform compat --help` still works and prints the one-line notice on stderr.
- `bashy conform --help` does not mention the batteries; `bashy selftest --help` does not mention the judge.
- Unit tests cover the five points above. Existing tests are updated, none deleted or skipped.

## Constraints
- Touch only `internal/agentos` (the files above and their tests), go.mod/go.sum for the one requirement, and the two release-bar docs. No change to the shell interpreter, coreutils or the suites themselves.
- Never build or test on the dev box. Claim a gate lane (`bashy claim elephant.lane3 --intent ...`), run there with `GOWORK=off`: `go vet ./internal/agentos/...` and `go test -count=1 ./internal/agentos/ -run 'Verify|Conform|Selftest|Commands|ReleaseBar'`, and release the lane. Do not run the full suite or `bashy dag test`.
- Do not push to main. Commit on your work branch with trailers `Sprint:`, `Story:`, `Story-ID:` for this story and `Agent:` with your registered name. Record the verify command and its result before you submit. The conductor reviews, gates and merges.
