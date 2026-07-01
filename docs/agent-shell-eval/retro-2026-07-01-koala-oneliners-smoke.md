# Retro - Koala Oneliners Smoke 2026-07-01

## What Worked

- Koala `oneliners` is a useful low-cost benchmark: it runs real pipelines, validates outputs, and surfaces embedded coreutils behavior.
- Running scripts manually avoided Koala runner assumptions about host `git` and package setup.
- The smoke completed in seconds and produced a clear differential: three scripts matched GNU byte-for-byte, one exposed a precise `grep` gap.

## What Should Improve

- Normalize Koala inputs the same way upstream does (`dos2unix`) before comparing against Koala reference hashes. For this smoke, GNU-vs-bashy byte equality was the more reliable comparator.
- Preserve only concise raw artifacts by default. The `.out` files are useful locally but too noisy for source control.
- Add a reusable runner script under `eval/` if Koala becomes a recurring benchmark.

## Bashy Follow-Ups

- Coreutils `grep` needs GNU-compatible BRE backreferences or an explicit fallback.
- The shell-check feature should flag unsupported embedded coreutils features before execution, especially for agentic tools.
- Once `grep` is improved, rerun the same four-script smoke and expand to all min `oneliners`.

## Conductor Follow-Ups

- Keep using small, discriminating benchmark slices rather than full benchmark suites first.
- When a benchmark fails, classify it as shell semantics, coreutils semantics, harness issue, or agent behavior before assigning fixes.
- Delegate independent coreutils gaps separately from shell-interpreter gaps; this one belongs in `../coreutils`, not `../sh`.
