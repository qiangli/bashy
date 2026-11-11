# Sprint 614 collaboration journey: evidence (S6)

One final pass on the Linux test host (WSL Ubuntu 24.04, Go 1.27.2) after
all Sprint 614 features landed. Hosts are named by role.

**Candidate:** bashy `b7d70e4c`, yoke `d0ba2a11`, outpost `fb334d0`,
cloudbox `07411af` (server and console). Repairs found by this pass are
listed under "Fixed by this pass" and were re-verified on the same host.

## Per-tool results

| Step | Tool / story | Result | Evidence |
|---|---|---|---|
| build | bashy | PASS | `GOWORK=off go build ./cmd/bashy` at the candidate |
| session-register-twice | Session (S5) | PASS | `examples/session/register.sh` twice in an isolated home; `session` registered, `tutor` absent |
| session-tests | Session (S5) | PASS | 65 Python tests + `server` `go test` |
| desk-roundtrip | Notepad, Viewer, Form, Chart | PASS | `bashy dag examples/desk/dag.md roundtrip` against real loopback servers |
| desk-parity-verbs | Whiteboard, Brainstorm, Form, Chart (S11) | PASS | per-app `go test ./...` incl. CLI parity and denied-access tests |
| app-ref-define | App refs (S2) | PASS | real notepad item resolves via `bashy define app:notepad/<id>`; dangling id, unlisted command and disabled allowlist (`BASHY_REFS_APP_COMMANDS=`) all refused |
| agentos-refs | App refs (S2) | PASS | `internal/agentos -run Ref\|Define\|SrcPin\|Suite\|Member` incl. `TestRefResolversCoverEveryKind`, `TestSrcPinsMatchLinkedModules` |
| kb-catalog | KB catalog (S3) | PASS | `examples/kb-catalog/tests/test.sh` |
| collab-save-lfs | Git save, publish, fresh clone, LFS (S4) | PASS (after fix) | `tests/roundtrip.sh` with git-lfs 3.4.1: no-smudge clone holds an LFS pointer, `git lfs pull` restores identical bytes; secret path and `../` target refused |
| video-test | Video | PASS | `examples/video/test.sh` (ffmpeg, asciinema, agg present) |
| record-contract | Recording | PASS | contract only; a take needs a person at a camera (human step) |
| transcribe-cli | Transcription | PASS | `bashy commands transcribe --features` |
| yoke-suites-circles | Suites and circles client + fake master (S9, S10) | PASS | `pkg/fleet/...`, `pkg/boundary/...`, `pkg/ref/...` |
| outpost-share-circle | `outpost apps share --circle` (S9) | PASS | `internal/agent/portal/...`, `cmd/outpost -run Share` |
| cloudbox server (S9) | circles, suites, live access check | PASS | graded on the Linux test host: `internal/api` Circle/Member/Share/Suite/CanReach/Satellite, `internal/db` schema rev 22, `internal/handlers`, `internal/server` routes |
| collaboration-e2e | Brainstorm + Whiteboard browser, Matrix | **FAIL (pre-existing)** | fails identically at bashy `10d55bf3` (before Sprint 614); tracked as todo `b29cbe96b2d3` |

## Fixed by this pass

- **collab-save did not apply Git LFS** (`cf8ff000`). Save ran `git lfs
  track` through system git but `add/commit/push` through bashy's built-in
  git, which runs no clean filter or pre-push hook, so media was committed as
  plain blobs. Writes now go through system git; publish pushes `HEAD`.
- **Round-trip test portability** (`2df155b7`): the LFS branch used a
  bashy-only git flag and `rg` under plain bash; it never ran on a host
  without git-lfs.
- Venue only (no code change): the test host lacked jq, ffmpeg, asciinema,
  agg, Node >= 20 for Playwright, Chromium and the pinned tuwunel; the first
  run's script also set a global `GOFLAGS=-mod=mod` (breaks the vendored
  shaped-app test) and had not registered notepad in the isolated home.

## Absent or deferred (not reported as passing)

- **Memos**: parked by operator decision (S7, `docs/memos-decision.md`).
- **Shared calendar**: decided, not built (S8, `docs/calendar-decision.md`).
- **Klatch server journey**: not exercised here (needs a Databag server);
  Klatch is covered by its own `tests/klatch-e2e` gate.
- **Live satellite suite → circle reach** (200/403/401 through a deployed
  master): verified at unit level on the server and against the fake master;
  a deployed end-to-end run belongs to the release E2E.
- **cloudbox unit tests on macOS**: `internal/api` imports the k3s server
  (flannel, kube-router are Linux-only); they run on the Linux test host.
