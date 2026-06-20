# Prior-art reference policy (POSIX shell conformance)

`priorart/` holds **local clones of other shells**, used to understand and — where
the license permits — adapt POSIX shell behavior while widening the conformance
corpus and fixing real `DEVIATION`s. It is **gitignored**: we never redistribute
upstream sources; we adapt *algorithms* into Go (in `../sh`) with attribution.
Mirrors the `coreutils/priorart/` model.

Interpreter fixes land in the **`sh`** fork (`interp`/`expand`/`syntax`); this repo
(`bashy`) owns the CLI + the conformance corpus/harness. So the workflow is:
harness (`scripts/posix-diff.sh`) surfaces a deviation → consult the references
below → implement the fix clean-room in `../sh` → re-gate (`make test-bash` 86/0 +
`posix-diff.sh` 0 deviations).

## Reference shells and what each is licensed for

| Shell | License | Read source to understand? | Adapt / translate code? | Role |
|---|---|---|---|---|
| **dash** | **BSD‑3‑Clause** | **yes** | **yes — with attribution** | strict‑POSIX reference |
| bash 5.3 | GPLv3 | yes (behavior understanding) | **no** (port forbidden) | bash‑fidelity target |
| yash | GPL‑2.0 | docs/manual + black-box oracle only | **no** | strictest‑POSIX oracle |
| mksh | (check) | oracle | only if permissive + attributed | Korn-lineage oracle |
| zsh | (check) | oracle | only if permissive + attributed | emulated-POSIX oracle |

**sh is itself BSD‑3‑Clause**, so adapting dash (also BSD) is license-compatible.
GPL sources (bash, yash) may be **read to understand behavior** — the project's
"check the bash 5.3 source" practice — but **never ported/translated** into the
permissive sh/bashy tree. The authoritative reference for *correctness* is always
the **POSIX standard** itself; the shells confirm behavior empirically.

## dash carve-out

dash is BSD **except `src/mksignames.c`**, which is GPL (FSF / GNU Bash). COPYING
notes it is *"not directly linked with dash; however, its output is"* — it is a
build-time signal-name-table generator, irrelevant to shell semantics. **Do not
read or adapt `mksignames.c`.** Everything else under `priorart/dash/src/` is
BSD‑3‑Clause.

## When adapting BSD code (dash) into `../sh`

1. Understand the algorithm from the BSD C; reimplement in Go (don't transliterate
   blindly — match sh's idioms).
2. Add a provenance note on the touched file/area: source repo + BSD‑3‑Clause +
   what was adapted.
3. Record it in `sh`'s `THIRD_PARTY_LICENSES.md` (or this repo's equivalent).
4. Ground the behavior in the POSIX spec so the result is demonstrably correct and
   independent.

## Setup

```sh
mkdir -p priorart && cd priorart
git clone --depth 1 https://git.kernel.org/pub/scm/utils/dash/dash.git dash
# (gitignored; for study/adapt — see table above)
```

dash is the strict‑POSIX reference; bash 5.3 (via the gitignored `external/bash-5.3`
symlink) is the bash‑fidelity reference; yash/mksh/zsh are black-box oracles in the
harness.
