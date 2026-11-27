# Shape gate evidence

The registered-app contract adds S-16 through S-32. The `cert` and `mutate` targets write its reports to `shape-cert.json` and `shape-mutate.log`. Run `alone` for its standalone lifecycle evidence.

## Earlier refusal-only contract (2026-11-27)

The archived files below cover S-01 through S-15.

Gate venue: the Windows gate host's WSL gate lane 1 (Linux 6.18 WSL2, Ubuntu-24.04, x86_64, Go 1.27.4), with `gate-linux` also claimed. The folder was copied to `/root/shape-alone/shape` on that host, outside any checkout, and driven only by its own lifecycle: `bashy dag -f dag.md source` (cloned the implementation at the certified commit), `build` (built bashy from that source and wrote the `shape` launcher) and `test`. Assertions ran with `--fresh-home` and an explicit `--user shape-tester`, so no registered app, real agent or live service was involved: sandbox-fake state only. The gate-host e2e reference runs this spec also reuses are the repository's `tests/group-shaping-e2e` and `tests/field-loop-e2e` (stub shaping agent, synthetic accounts, fake upstream).

Gate logs (raw, from the gate host):
- `gate-source.log`, `gate-build.log`, `gate-test.log`: the folder-alone lifecycle; `gate-test.log` holds the cert JSON with `"verdict": "PASS"` and `"noisy": []` over 3 runs, all 15 assertions PASS in every run.
- `cert-gate.log` / `cert-gate.json`: `conform cert --runs 3 --fresh-home` against the gate-built `shape` launcher — PASS, `noisy: []`, environment linux/amd64, go1.27.4.
- `mutate-gate.log`: `conform mutate --fresh-home` against the gate-built launcher — strength 27/27, every mutant killed, no survivors to record.

Dev-host logs (kept for comparison; they do not satisfy the gate acceptance on their own):
- `cert.log` / `cert.json`, `mutate.log`: the same cert (PASS, 3 runs, none noisy) and mutate (27/27) on the dev host (darwin/arm64) through the installed bashy binary.

Also verified at the delivery commit: `bashy conform spec check shapeware/shape` reports nothing and `bashy --bashsharp shapeware/index.bsh check` prints `PASS shapeware/shape`.

Two defects were found by the gate run and fixed in this spec (no product code changed): the lifecycle steps used `$PWD`, which a gate shell may inherit from another host, so they now resolve the folder with `pwd -P`; and the assertions assumed an ambient principal, so every `shape` command now passes `--user shape-tester` explicitly.

Unmeasured: any shaping of a registered app with a real (non-stub) agent. See the known issues in `../SKILL.md`.
