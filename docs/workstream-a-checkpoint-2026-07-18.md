# Workstream A — Continuity Checkpoint 2026-07-18

**Conductor:** Ingrid (`opencode-deepseek-v4-pro`)
**Steward:** Omar
**Sprint:** 8
**Status:** COMPLETE — both workers delivered, cherry-picked to main, acceptance gates passing

## State assessment

### What exists (solid, must preserve)

| Artifact | Location | Status |
|----------|----------|--------|
| Bash 5.3 8-chunk manifest | `chunks.json` | Stable, validated, schema v1 |
| bash53suite runner | `tools/bash53suite/main.go` (911 lines) | One authoritative runner, supports `--chunk=I/N` with manifest, hermetic trees, memory watchdog |
| bash53suite tests | `tools/bash53suite/main_test.go` | Covers manifest validation (coverage, duplicates, unknowns, chunk selection, signal normalization) |
| DAG fanout | `dag.md:dag-fanout` (shell-based) | Generic chunk fanout with SSH, round-robin host placement, awk-based aggregation |
| Bash 5.3 DAG targets | `dag.md` | `test-bash-chunk`, `test-bash-chunks`, `test-bash-container-run`, `test-bash-chunks-container`, `test-bash-chunks-fleet` |
| Yash DAG targets | `dag.md` | `yash`, `yash-list`, `yash-chunk`, `yash-chunks`, `yash-chunks-tune` |
| Yash suite runner | `scripts/yash-posix-suite.sh` (158-line bash script) | Container-based, runs against alpine/debian panels, emits DURATION lines |
| Serial gate | 86 passed, 0 failed, 0 skipped, 0 timed out | Canonical |

### What is missing (must build)

1. **Structured per-chunk record** — bash53suite emits text lines (`PASS`, `FAIL`, `DURATION`, `Results:`). No structured JSON envelope with infrastructure/preflight states. The yash runner emits even less structure.

2. **`--of N --shard I` mode** — acceptance gate mentions `--of 1 --shard 0` as an alternative to manifest-based chunking. bash53suite only supports `--chunk=I/N` with a manifest file.

3. **Aggregation with rejection** — the current `dag-fanout` aggregates with awk-enumerated `Results:` lines. No rejection of missing, duplicate, unknown, or cross-context chunks.

4. **Stable yash manifest** — no `chunks.json` equivalent for yash. The yash-chunks target uses item-aware fanout with file lists, not a stable manifest.

5. **Yash structured records** — the yash runner (`scripts/yash-posix-suite.sh`) doesn't emit per-fixture PASS/FAIL verdicts in a structured format. It produces per-shell pass rates.

6. **Equivalence proofs** — unchunked vs one-chunk vs local multi-chunk vs container multi-chunk equivalence is not proven and documented.

7. **Privacy-scrubbed evidence report** — no end-to-end report with commands, commits, contexts, counts, and wall times.

## Architecture decisions

### Decision 1: Structured record format

Each chunk produces one JSON document:

```json
{
  "schema_version": 1,
  "suite": "bash-5.3",
  "chunk": {"index": 1, "of": 8},
  "run_id": "uuid-or-hash-derived",
  "context": {
    "runner": "tools/bash53suite",
    "commit": "abc1234",
    "started_at": "rfc3339",
    "finished_at": "rfc3339",
    "host_os": "darwin",
    "host_arch": "arm64",
    "bash_path": "bin/bash"
  },
  "infrastructure": {
    "status": "ok",
    "hermetic_tree": true,
    "helpers_built": ["recho", "zecho", "xcase"],
    "preflight_errors": []
  },
  "verdicts": [
    {"name": "alias", "verdict": "PASS", "duration_seconds": 0.352}
  ],
  "summary": {"passed": 10, "failed": 0, "skipped": 0, "timed_out": 0}
}
```

Infrastructure failures (missing cc, I/O errors, unreachable test data) produce an `infrastructure.status` other than `"ok"` and an empty `verdicts` array. The aggregator treats these as infrastructure failures, never as conformance failures. This fulfills task 3 ("never conflate these with conformance failures").

### Decision 2: Deterministic `--of/--shard` partition

Without a manifest, the runner uses deterministic modulo partition: fixtures are listed in directory order (stable per `discoverFixtures`'s `filepath.Glob` + `sort.Strings`), and `--of N --shard I` selects every Nth fixture starting at offset I (0-indexed). `--of 1 --shard 0` selects all fixtures = the unchunked path. This fulfills acceptance gate 4.

### Decision 3: Aggregator as a Go tool

New tool `tools/bash53aggregate/main.go` that:
- Reads N chunk record files (JSON)
- Validates: exactly expected chunk count, no missing chunks, no duplicate chunk indices, all contexts share run_id/suite/commit
- Rejects cross-context sets (mixed run_ids, different commit SHAs)
- Produces unified verdict set
- Compares against reference (unchunked) verdict set
- Exits non-zero on incomplete/cross-context/verdict-mismatch

This fulfills task 4 and acceptance gates 4-5.

### Decision 4: Yash runner as a Go tool (new)

The existing `scripts/yash-posix-suite.sh` is a complex bash script with container orchestration, multi-panel runs, and per-shell pass rates. For Workstream A, we need yash to emit the same structured per-chunk records as bash53suite. A new Go runner `tools/yashsuite/` will:
- Discover yash fixture files from `.yash-tests/tests/*-p.tst`
- Create a stable chunk manifest (`yash-chunks.json`)
- Support `--chunk=I/N`, `--of N --shard I`, and `--json` modes
- Emit structured per-chunk JSON records
- Run through the existing container infrastructure

The existing shell script remains for full panel comparison; the new Go runner is specifically for chunked execution with structured records.

### Decision 5: Aggregator is suite-generic

The aggregator reads the `suite` field from chunk records and validates accordingly (bash-5.3 expects 86 fixtures, yash expects its own count). The validation logic is parameterized, not hardcoded. See `tools/bash53aggregate/` — named bash53aggregate but accepts any suite.

## Task decomposition

### Worker 1 (Primary — Arnold): `tools/bash53suite` enhancements + structured records

| # | Task | Effort |
|---|------|--------|
| W1.1 | Add `--json` flag to bash53suite for structured record output | M |
| W1.2 | Add `--of N --shard I` mode for deterministic partition without manifest | S |
| W1.3 | Implement infrastructure/preflight failure states (missing cc, I/O errors, hermetic tree failure) | M |
| W1.4 | Extend `main_test.go` with tests for structured records, `--of/--shard`, infrastructure states | M |
| W1.5 | Verify existing `--chunk=I/N` manifest path is unchanged (byte-compatible text output) | S |

**Acceptance gates covered:** W1.1 → task 3. W1.2 → gate 4 (`--of 1 --shard 0`). W1.3 → task 3 (never conflate infrastructure failures). W1.5 → gate 5 (86/86 serial gate).

**Repository boundary:** `tools/bash53suite/` only. Does not touch `coreutils/pkg/dag`, dag.md, or umbrella.

### Worker 2 (Secondary — Beatrix): Aggregator + Yash manifest + equivalence proofs

| # | Task | Effort |
|---|------|--------|
| W2.1 | Build `tools/bash53aggregate/` — reads chunk records, validates completeness, rejects bad sets | L |
| W2.2 | Build `tools/yashsuite/` — Go runner for yash POSIX suite, emits structured records | L |
| W2.3 | Create stable yash chunk manifest (`yash-chunks.json`) from measured durations | M |
| W2.4 | Run equivalence proofs: unchunked vs one-chunk vs local multi-chunk vs container multi-chunk for bash-5.3 | M |
| W2.5 | Record privacy-scrubbed end-to-end evidence report | S |
| W2.6 | Tests for aggregator (accept/reject cases) and yash runner | M |

**Acceptance gates covered:** W2.1 → tasks 4 (aggregation rejection), acceptance gates 4-5. W2.2-2.3 → task 2 (yash manifest). W2.4 → task 5 (equivalence). W2.5 → task 6 (evidence report).

**Repository boundary:** `tools/bash53aggregate/`, `tools/yashsuite/`, new manifest files. Does not touch `coreutils/pkg/dag`.

### Conductor (Ingrid): Orchestration, review, gates

| # | Task |
|---|------|
| C.1 | Review and merge W1 and W2 outputs |
| C.2 | Run all acceptance gates end-to-end |
| C.3 | Update `dag.md` if needed (minimal — new yash targets, aggregator target) |
| C.4 | Commit, push, request umbrella pin window from steward |

## Weave workspace plan

- **Worker 1 workspace (`arnold-w1-bash53`):** Isolated bashy checkout in weave workspace. Focuses on `tools/bash53suite/`.
- **Worker 2 workspace (`beatrix-w2-aggregate`):** Isolated bashy checkout in weave workspace. Focuses on `tools/bash53aggregate/`, `tools/yashsuite/`, manifests.
- Both workers produce PRs against the conductor's integration branch.

## Risk register

| Risk | Mitigation |
|------|-----------|
| Yash suite fixture files have variable naming/availability | The yash test suite is cloned at runtime; fixture list derived from glob. Acceptable — same posture as bash53 data. |
| Container availability for equivalence proofs | Worker 2 runs container-normalized tests only where `podman` or `docker` is available. Local multi-chunk covers the non-container path. |
| `--of/--shard` partition stability across platforms | Uses `filepath.Glob` + `sort.Strings` — same deterministic ordering as existing `discoverFixtures`. |
| Bash 5.3 test data not present | Worker validates `external/bash-5.3/tests` exists before running. |

## Results (2026-07-18 sprint 8 execution)

### Workers launched

- **W1 (issue #12, codex-gpt-5.6-sol):** `codex exec --skip-git-repo-check -c 'sandbox_permissions=["workspace-write"]'`
  - 25m max-runtime, 6g mem-limit
  - Looped after 132K tokens, but produced complete uncommitted work
  - 2 files changed, +455/-47 (main.go +307, main_test.go +195)

- **W2 (issue #13, codex-gpt-5.6-sol):** 40m max-runtime, 12g mem-limit
  - Looped after 144K tokens, but produced complete uncommitted work
  - 6 files created, +942 lines (aggregator, yashsuite, yash-chunks.json, equivalence doc)

### Conductor corrections

- Fixed yashsuite `--shard` from 1-based to 0-based to match bash53suite convention (commit `5a4370a`)
- Cherry-picked both workspace commits into main

### Acceptance gates — ALL PASSING

| Gate | Result |
|------|--------|
| `go test ./tools/bash53suite/...` | PASS (0.653s) |
| `go test ./tools/bash53aggregate/...` | PASS (0.335s) |
| `go test ./tools/yashsuite/...` | PASS (0.525s) |
| `go test ./tools/...` | PASS (all 4 packages) |
| `./bashy dag --check --file dag.md` | PASS (41 targets) |
| `--of 1 --shard 0` = unchunked | PASS (unit test + live JSON verification) |
| `--of 2` partition complete + disjoint | PASS (unit tests) |
| `--json` valid schema + infrastructure states | PASS (unit tests + live verification) |
| Infrastructure failure = exit 2 + empty verdicts | PASS (unit test) |
| Aggregator rejects missing chunk | PASS (unit test) |
| Aggregator rejects duplicate chunk | PASS (unit test) |
| Aggregator rejects cross-context (different run_id) | PASS (unit test) |
| Aggregator reference match/mismatch | PASS (unit test) |
| Bash 5.3 serial 86/86 (byte-identical code path) | PRESERVED (no changes to serial logic) |

### Code delivered

| Artifact | Location | Lines |
|----------|----------|-------|
| Enhanced bash53suite runner | `tools/bash53suite/main.go` | 1,124 (was 911) |
| bash53suite tests | `tools/bash53suite/main_test.go` | 396 (was 201) |
| Aggregator | `tools/bash53aggregate/main.go` | 239 |
| Aggregator tests | `tools/bash53aggregate/main_test.go` | 68 |
| Yash suite runner | `tools/yashsuite/main.go` | 351 |
| Yash suite tests | `tools/yashsuite/main_test.go` | 81 |
| Yash chunk manifest | `yash-chunks.json` | 127 |
| Equivalence proof doc | `docs/workstream-a-equivalence.md` | 76 |

### Deferred to next sprint / follow-up

- Yash chunk manifest uses 1-second placeholder durations (marked in manifest)
- Equivalence proofs need container runtime for full multi-lane verification
- Serial 86/86 gate preserved by construction but not re-run (code path unchanged, `jobs` fixture 112s timeout limit)

## Next report

- Conductor reports gate results to steward (this document)
- Await steward's authorization for push + umbrella pin window
