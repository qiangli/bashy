# Sprint 561 A4a: frozen-candidate public-lane results

Story `9f27d68da431` (#3061), child of A4. Candidate: bashy `f1bcd711`
(sh pin `74586fde5f24`, coreutils pin `211789fadf75`, both corroborated in
`go.mod`). Standalone, `GOWORK=off` everywhere. Darwin/arm64,
`go version go1.27.2 darwin/arm64`. No code changes; failures are reported,
not fixed. Each lane ran ONCE, with existing bounds (no skip/timeout growth).

This report supersedes the prior A4a attempt (`a4b663dd`), whose `go test`
run was invalidated by host disk exhaustion (46 ENOSPC errors) and which left
lanes 2 and 4 undone. It ran here with 21–36 GB free on
`/System/Volumes/Data`; all four lanes executed to completion.

## Candidate identity vs checkout

Checkout HEAD at run time: `ecf5fff2`. `git diff f1bcd711 HEAD -- go.mod
go.sum Makefile` is empty, so every Go build input equals `f1bcd711`. The only
non-`docs`/non-`KB.md` delta is three cert/manifest scripts from `a4f5e7bb`
(new `scripts/cert-closure.py`, `scripts/cert-identity-check.sh`, +1 line in
`scripts/s561-baseline-manifest.sh` adding the `cert_closure` key). None is a
Go build input; the manifest tool change is reported, not reverted.
(`KB.md` carries another agent's uncommitted note; untouched, not a build input.)

## 1. `GOWORK=off go test -timeout 20m ./...` — exit 1, VALID run

Log `/tmp/a4a-verify/gotest.log` (out of git), sha256
`6f71323b37122053e47abc5fae1e2807628f799b9ae033b8e3a1c22e8cae85d7`
(48 lines). 43/43 packages: 31 ok, 1 FAIL, 11 no-test-files. No skips, no
panics, no per-package timeout. All results freshly executed (real durations,
nothing `(cached)`). Slowest: `internal/agentos` 600.4 s, `examples/desk`
322.4 s, `cmd/bashy` 148.7 s — inside the existing 20 m bound.

FAIL packages and failing tests (top-level): exactly one —
- `cmd/bashy` — `TestYAMLAgentFenceCompilerRunner`
  (`polyglot_agent_test.go:64`): opens
  `.../workspaces/ycode/examples/agent.yaml`, missing in this checkout.

Classification vs A2 baseline (`docs/s561/a2-results.md` §4.1):
- F1 `TestYAMLAgentFenceCompilerRunner` — **still pre-existing** (same test,
  same cause: absent sibling `ycode` workspace is a setup prerequisite, not a
  product defect).
- F2 `TestCommandDocSourceNames` — **fixed** (`internal/agentos` ok).
- F3 `TestRunSignaledIsNonLossy` — **fixed** (`internal/agentos` ok).
- F4 `TestSrcPinsMatchLinkedModules` — **fixed** (`internal/agentos` ok).
- F5 `tools/bash53suite` gate-tooling TIME — **fixed** (package ok, 30.8 s).
- **NEW: none.** Zero failures outside the single F1 setup item.

## 2. `GOWORK=off make dist` — exit 0

6 platforms x 2 entries = 12 binaries in `bin/dist/` (gitignored; removed
after hashing to relieve shared disk — hashes below are the record).
`PLATFORMS = linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
windows/arm64`; every `bashy` binary passed `elfaudit` + `releaseeligibility`
during the build (see `/tmp/a4a-verify/dist.log`).

| Binary | sha256 |
| --- | --- |
| bash-darwin-amd64 | `644bd4ddda0df2979c1662544b59e837bdbb77241b91f297520a00331cf4d581` |
| bash-darwin-arm64 | `9bb41bd826854a423675b364b743ecfdc2e82cbc0c26cf746dd2d606c32e6bd4` |
| bash-linux-amd64 | `186ddf409172510cb6cc8f07021441dafb3fb3f15982ddaf3fb7e5b4da7c67c1` |
| bash-linux-arm64 | `370336a7f04e25bcefedc4bce06a3222dffa0c690223138dcbef8c7d0ebf3362` |
| bash-windows-amd64.exe | `6d0bba97913ee3dc21e6506311fd2f557aa07fcfc492980f50979bc9a6eaf203` |
| bash-windows-arm64.exe | `d1a71869ad8f99f5fffc40d77b7b4545469abd09b0ada524aeca46983de12c54` |
| bashy-darwin-amd64 | `cd5f767545f18e328e39dc85071fa0d7fcf38f7e7f9b28dbe76295f3f0c785b5` |
| bashy-darwin-arm64 | `ed8d2d049e21b7b8fe42686567050d99cb1e7c13f9701843bdd787a2ece2152a` |
| bashy-linux-amd64 | `4d322738409f7633ee93976d556ee82ed91e66ce432445947dc2a74d26004179` |
| bashy-linux-arm64 | `5bd7ba15666a6efa454696a61292ef4853bc3283d03dc5cd2a992f9f6271984d` |
| bashy-windows-amd64.exe | `aa6195cccff9a4c9643a9da672d377dc9bca03e6a82ebc800d7dc1a320364c9c` |
| bashy-windows-arm64.exe | `116d092acc019059f38654f4b3e3d7be93b29687ae9dfee562ab9be6780d83fd` |

## 3. Candidate manifest — DONE

`GOWORK=off make build` exit 0 (`elfaudit` and `releaseeligibility` PASS;
`/tmp/a4a-verify/build.log`), then `GOWORK=off sh
scripts/s561-baseline-manifest.sh f1bcd711` ->
`docs/s561/candidate-manifest.json` (schema `bashy.s561.baseline.v1`,
`source.commit f1bcd711`; includes the `cert_closure` key from the HEAD
tooling noted above). Artifact sha256:
- `bin/bash` `b0d19ed071ba4bd93811ba5aba26b8a5bf7aa82e3e920ebf1b720a713d575efc` (identical to the A1 baseline wrapper — wrapper unchanged).
- `bin/bash.real` `206ae8f80ccd542213fe9bd2446bcdcd39c1229a5307e714851698bdfddfe77a`.
- `bin/bashy` `81e616813181762ba992b10c60fa9e311fad9033e3a005359e8f740827976da5`.
- `bin/bashy.real` null (absent by Makefile design, same as baseline).

(`bash.real`/`bashy` differ from the A1 baseline hashes as expected: the
candidate bumps the sh pin and build stamps `buildID f1bcd71` /
`ShellRuntimeCommit ...74586fde5f24`. They also differ from the superseded
attempt's manifest for a benign, verified reason: the binaries embed a Go VCS
stamp (`go version -m bin/bashy` here reports
`vcs.revision=ecf5fff28fe9a59e899b4baf40950a0b4e10f425`,
`vcs.modified=true` from the other agent's uncommitted `KB.md` note), so
hashes are checkout-stamp-sensitive while all compiled content equals
`f1bcd711`. The stamp-free `bin/bash` wrapper is byte-identical to baseline.)

## 4. Container gate — runtime NOT usable; exact blocker + native fallback

`make test-bash-container` was not run: no container runtime is usable on this
host. No system `podman` or `docker` on PATH; `bashy podman info` reports
`provider: libkrun, version 6.1.2` but cannot connect (`dial unix
.../podman.sock: connect: no such file or directory`); `bashy podman machine
list` shows zero machines and `system connection list` is empty. Provisioning
a new VM plus pulling the pinned Ubuntu base image would be new
host-exclusive setup, out of bounds for this story — no machine was created
or started. This is a setup blocker, not a candidate verdict.

Fallback (non-authoritative): `GOWORK=off make test-bash` exit 0 —
**86 passed, 0 failed, 0 skipped, 0 timed out**
(`/tmp/a4a-verify/test-bash.log`). Native macOS execution, not the hermetic
Linux image, so it does not substitute for the authoritative container gate;
the container lane stays open for a provisioned host.

## Ownership and hygiene

- No source, test, timeout, or skip was changed; `git status` shows only the
  two deliverable files touched by this story (plus another agent's
  pre-existing uncommitted `KB.md` note, left alone).
- Raw logs stay out of git under `/tmp/a4a-verify/` (`gotest.log`,
  `dist.log`, `dist.sha256`, `build.log`, `test-bash.log`,
  `manifest.err`); `bin/dist/` (~1.1 GB) was removed after hashing. No other
  agent's files were touched.
- A4 acceptance is unchanged; this story delivers only the four public lanes
  above. Open item for A4: the authoritative `test-bash-container` hermetic
  gate still needs a provisioned host.
