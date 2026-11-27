# Evidence (Sprint 737 L1)

Gate run on the secondary gate host's Linux VM, 2026-11-27, through the
standard dag targets (`bashy dag -f dag.md source|build|check|cert|mutate|alone`),
reference bashy built from the certified commit. Contract: S-01..S-33
(S-01..S-15 refusals, S-16..S-33 the registered fixture app's loop: add,
wish, show, diff, verify, preview, approve, name, undo, restore, scope).

| Target | Result | Wall |
|---|---|---|
| `source`, `build` | clone at the pin, build ok | 18 s |
| `check` | `conform spec check` clean | <1 s |
| `cert` | PASS, 3 runs, none noisy (`shape-cert.json`, contract sha256 `136ab591…` = this CONTRACT.md) | 17 s |
| `mutate` | strength 123/123 (`shape-mutate.log`) | 672 s |
| `alone` | lone copy: source → build → test, S-01..S-33 PASS (`alone-gate.log`) | 23 s |

`dag: effect cap` lines are warnings only. Earlier rounds (refusal-only
contract; 23 survivors behind cat/jq wrappers) are in git history.
