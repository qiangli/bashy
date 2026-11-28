# Evidence (Sprint 744 N4)

Gate run on the Windows test host's Linux lane, 2026-11-28, through the
standard dag targets (`bashy dag -f dag.md source|build|check|test|cert`).
The reference is bashy built by the `build` step from the commit in
`SKILL.md` (`metadata.reference-commit`), driven through the `session`
launcher that step writes; the cert report's digest is the launcher's.
Contract: A-01 to A-24 (no A-11): A-01 to A-09 and A-12 to A-13 the named
sessions, their claim and their mail; A-14 to A-19 and A-22 the refusal and
the callers left alone; A-20 and A-21 the `bashy <tool>` session form; A-10,
A-23 and A-24 cited go tests.

| Target | Result | Wall |
|---|---|---|
| `source`, `build` | containing repository, build ok | 27 s |
| `check` | `conform spec check` clean; index check PASS for both surfaces | <1 s |
| `test` | A-01 to A-24, 23 of 23 PASS | 37 s |
| `cert` | PASS, 3 runs, none noisy (`agent-identity-cert.json`, contract sha256 `1842377c…` = this CONTRACT.md) | 138 s |

`mutate` and `alone` results are added when their runs finish. `dag: effect
cap` lines are warnings only.

Venue: Linux amd64, Go 1.27.4 on `PATH` with a module cache that already
holds the implementation's dependencies (`bashy mb` and `bashy inbox`
compile their adapter on first use; with the cache passed through, a fresh
HOME downloads nothing). No network is needed with a warm cache.
