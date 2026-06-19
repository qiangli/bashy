# Plan: POSIX conformance for bashy

Status: **planning / Phase 0–1 kickoff** (2026-06-19).

## The reframe: conformance ≠ behavioral parity

The bash-5.3 campaign (86/86 fixtures) gave bashy **behavioral parity** — it
*mimics GNU bash*. POSIX **conformance** is alignment with the formal **IEEE
Std 1003.1 (POSIX.1-2017) XCU** spec. Three facts shape this work:

1. **Bash is not POSIX-conformant by default** — it ships GNU extensions and
   non-POSIX behaviors on purpose. It has a **POSIX mode** (`set -o posix`,
   `--posix`, or invoked as `sh`) that flips **76 documented behaviors**
   (`doc/bashref.texi` → "Bash POSIX Mode") toward the standard. So
   "conformance for bashy" means conformance **in POSIX mode**, and the natural
   first target is **parity with bash's POSIX mode**.
2. **Scope: shell vs utilities.** POSIX XCU (and the VSC-PCTS) tests **both**
   the shell language **and ~160 standalone utilities** (`ls`/`grep`/`sed`/
   `awk`/…). bashy is a **shell**: it owns the `sh` language + POSIX
   **built-ins** (`cd`, `read`, `getopts`, `printf`, `test`/`[`, `export`, …),
   not the standalone utilities. bashy's conformance scope = the `sh` utility +
   its builtins; utility tests exercise the host's tools. **Scope this, or a
   VSC-PCTS run mostly tests the wrong binaries.**
3. **`coreutils` (pure-Go POSIX utilities) is a sibling in the umbrella.** The
   fuller "Shell AND Utilities" story is **bashy + coreutils** — a separate,
   later track.

## Phases

### Phase 0 — Scope & baseline (cheap, first)
- Target: POSIX.1-2017 §2 Shell Command Language + the special/regular
  built-ins bashy implements; asserted **in POSIX mode**; standalone utilities
  out of bashy's scope (→ coreutils track).
- Baseline: diff `bashy --posix` vs `bash 5.3 --posix` over a POSIX corpus
  (reference bash via `docker run bash:5.3`, since macOS ships bash 3.2).

### Phase 1 — Bash POSIX-mode parity (highest ROI; free; proven machinery)
- Extract the **76 documented POSIX-mode behaviors** (`bashref.texi`) into a
  checklist (`docs/posix-mode-behaviors.md`) — the authoritative map.
- One fixture per behavior asserting `bashy --posix` == `bash --posix`.
- Fixes land in **sh** (already has `LangPOSIX`/`optPosix`), measured by bashy
  — the **bash-5.3 campaign machinery** (weave/conductor, a POSIX scoreboard,
  gate on no-regression to the 86/86 + CI green).

### Phase 2 — Open POSIX shell suites (free, no TET3)
- Run open corpora against `bashy --posix` before the formal cert:
  **yash** (most POSIX-conformant + thorough tests — best reference), **dash**
  (near-POSIX proxy), **modernish** feature tests, **Austin Group**
  defect/interpretation cases. Triage → fix in sh.

### Phase 3 — Official VSC-PCTS2016 (formal certification)
- Apply for the free open-source license from The Open Group
  (https://posix.opengroup.org/testsuites.html) → build **TET3 + VSXgen** →
  configure `sh`→bashy → run the **shell + builtin** subset (not full
  utilities) → parse journals (PASS/FAIL/UNSUPPORTED/UNTESTED/FIP) → triage.
- **After Phases 1–2** — don't start the 12-month license clock or the heavy
  TET3 build until bashy is already close.

### Phase 4 — Fix loop (throughout)
- Deviations → issues → fix in sh → gate on the bash-5.3 suite (no regression)
  + new POSIX assertions + `go test` + CI green. POSIX scoreboard alongside the
  bash-5.3 one.

## Caveats
- **Pick the target:** "parity with bash's POSIX mode" (Phase 1, achievable
  like bash-5.3) ≠ "100% VSC-PCTS PASS" (certification-grade; even bash
  deviates in edge cases). Former = sprint-series; latter = multi-month.
- **bashy alone can't be "Shell *and* Utilities" conformant** — needs the
  coreutils track too.
- **Effort:** Phase 1 ≈ the bash-5.3 campaign's size; Phase 3 is substantial.

## Sequence
0 → 1 → 2 → 3, fix-loop throughout. Start with **0 + 1** (free, proven
machinery, clearest payoff: true bash-POSIX-mode parity via the 76-item map).
