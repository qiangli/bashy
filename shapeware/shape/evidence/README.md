# Evidence (2026-11-27)

Venue: this host (`dragon`, darwin/arm64) using the installed bashy binary (build 345f01d) through a `shape` shim (`exec bashy shape "$@"`), with `--fresh-home` per run and no registered app, so every assertion runs with sandbox-fake state only. No product code was built or changed. The gate-host e2e reference runs this spec reuses are the repository's `tests/group-shaping-e2e` and `tests/field-loop-e2e` (stub shaping agent, synthetic accounts, fake upstream).

- `cert.log` / `cert.json`: `bashy conform cert --contract CONTRACT.md --reference-cmd <shim>/shape --candidate-cmd <shim>/shape --runs 3 --dir shapeware/shape --fresh-home` — PASS, 3 runs, `noisy: []`, all 15 assertions PASS in every run.
- `mutate.log`: `bashy conform mutate CONTRACT.md --path <shim> --dir shapeware/shape --fresh-home` — strength 27/27, every mutant killed, no survivors to record.
- Folder-alone: `shapeware/shape/` copied to `/tmp/shape-alone/shape` (outside any checkout) passed `bashy conform spec check` (no output) and the same 3-run cert (PASS, kept at `/tmp/shape-alone/alone.log` on this host).
- `bashy conform spec check shapeware/shape` reports nothing; `bashy --bashsharp shapeware/index.bsh check` prints `PASS shapeware/shape`.

Unmeasured: the `build`, `install` and `run` lifecycle steps on a Linux gate host (they build bashy from source), and any shaping of a registered app with a real agent. See the known issues in `../SKILL.md`.
