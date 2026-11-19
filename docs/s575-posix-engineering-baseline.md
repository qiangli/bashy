# Sprint 575: separated-source POSIX engineering baseline

The full Profile D run completed on 2026-11-19. This records a reproducible
engineering regression baseline after repository separation. Official POSIX
certification, authority rulings and manual dispositions remain open.

## Result

The original configured runner executed all 117 targets, retaining 117 journals
and exactly 9,337 execution results. First licensed TP: 15:24:46 UTC; final
target completion: 21:59:42 UTC. All target processes exited zero with no caps.
Process success does not mean every test purpose passed.

| Metric | Result |
| --- | ---: |
| Execution test purposes | 9,337 |
| Certification PASS group | 9,270 |
| FAIL | 4 |
| UNRESOLVED | 54 |
| INSPECT, requiring manual decisions | 9 |
| UNINITIATED / UNREPORTED | 0 / 0 |
| Timeouts / capped targets | 0 |

PASS group includes the original applicable UNSUPPORTED, UNTESTED and NOT IN
USE dispositions. It does not mean 9,270 literal PASS results. The original
parser separately excluded 178 build/setup records.

The residual counts and observed families match the
[2026-11-04 S561 engineering baseline](s561/a4-licensed-results.md):

- Four `more` FAIL results: TPs 130, 132, 133 and 134.
- Fifty-three UNRESOLVED results from empty-target symlink setup.
- One `man` UNRESOLVED result from its command-count precondition.
- Nine prescribed INSPECT results: `stty` seven, `locale` one, `df` one.

All 67 non-pass/manual results remain recorded. None was waived, relabeled or
fixed by changing a licensed test. Existing authority and manual-resolution
work continues separately. This is not a clean certification pass.

The shell scenario completed with 493 PASS-group outcomes and zero blockers.
The full run's mailx, at, batch and crontab targets completed without their
earlier orchestration-related failures. These observations belong to the
exact candidate below, not arbitrary later heads or a combined Bashy binary.

## Source marker and artifact identity

Annotated marker: **`posix-baseline/s575-2026-11-19`**. It identifies the
tested source snapshot, independently of RC38 or any product release.

| Repository / role | Exact commit |
| --- | --- |
| Umbrella approval snapshot | `295cd32d032fb94dda2c032c26109032bd5af4b8` |
| Bash, strict `cmd/sh` | `7855c2f038f2c8ccba1baf8478b0b8fb3dcd8cd2` |
| Coreutils | `c656935287fa4bff4920b08e41156d13d7c5702f` |
| Shared Bashsharp CLI dependency | `4141cc07cfdc6c0a838825485ef41e54a55fa9a0` |
| Syntax-only harness tool | `fbb54d3f9dee6a1b4f8364ba02d82873df119a18` |
| Private harness | `6966bc6458e862d7ae03a4a01e7044ab842399ba` |

The marker is published in the umbrella, Bash and Coreutils repositories at
their respective commits above. The umbrella's Bashy pin is syntax-only tooling
for this run, not a release-candidate product pin. The existing shared engine
and CLI remain pinned; their further separation is deferred.

- Artifact manifest SHA256:
  `6dde18df80537f6c4be1596222683d69c8a98c660a117474844d1530f1891138`.
- Build receipt SHA256:
  `d7fba64b230cdd44084cfdb04832f7ffb3912c7c5554059d793531df7791381a`.
- Bash launcher SHA256:
  `28d1e291a72c7ef90c2a9d32558845a72c5eccbc5212e839e24821540eadeffb`.
- Strict shell payload SHA256:
  `28025509e95a5604134bb44a9ccc2ec28372b426ce48f1995f0a7e244d4ec141`.
- Coreutils payload SHA256:
  `d9361975eb80b75f4631b1231a1bee77e3a3997bd208ef3b70013bc426e60c8a`.
- Builder image digest:
  `sha256:c99fcb7318b23ed06b3775368ee3966795a5498fb9a050a92d6f21866680b7ba`.

Build context: Go 1.27.2, `GOWORK=off`, Linux amd64. Coreutils used the existing
`bashy_cert,bashy_cert_base,osusergo` tags and static linking. Its executed
recipe bound `pkg/schedule.shellPath` to `/vsc/sut/sh` and
`cmds/crontab.defaultCronPATH` to `/opt/bashy/bin` through their full-package
`-X` flags. The latter has no host PATH tail. Hashed executed recipes and
explicit flag receipts retain these bindings; trimpath build info omits them.

The declared base provider inventory remained 102 targets: 92 Go utilities,
10 shell commands, zero external utility providers. Original applicability
and 14 optional exclusions stayed unchanged. Native build tools and the
syntax-checking tool were separately recorded harness infrastructure.

## Retention and closeout

The original full-run retrieval completed with its existing guards unchanged.
Before/after/final checksums, identical start/end inputs, all 117 journals,
23 public summary files and 12 stage files were verified. Independent local
recomputation using the original parser exactly reproduced the recorded tally.

Restricted evidence remains in the maintainer's private evidence store; no
licensed source, journals or private infrastructure details are published.
Sealed archive SHA256:
`63cd77b944361e999652210f30c39d5f74e5785921b6df2384b6d7c721405fe1`.
Archive readback verified all 196 regular files and all 195 checksum rows.

After supervisor review, the owned disposable test venue was retired through
the existing guarded procedure. Fresh provider API verification confirmed its
absence and that unrelated resources were unchanged. No test rerun, assertion
change, result editing or source mutation occurred during retention/cleanup.

For future measurements, use the
[full Profile D runbook](kb/pages/full-profile-d-posix-runbook.md), obtain the
licensed kit through its application links, and retain a new candidate's own
source/artifact/venue evidence. This baseline does not replace those checks.
