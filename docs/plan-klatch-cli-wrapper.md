# Plan: `bashy klatch` participant CLI wrapper (S597 / Story 3197)

- `examples/klatch/klatch.bsh` keeps `setup|provision|status CONFIG.json`
  (provision.py) and forwards every participant verb of the Klatch `klatch`
  executable (app/client/cli) unchanged: stdin/`--input`, `--session`, `--json`,
  `{ok,result}` / `{ok:false,error:{code}}`, `watch` JSONL, and the
  `[Assistant: NAME]` prefix (applied by the CLI, not re-implemented).
- CLI located via `KLATCH_CLI` (`*.js` run with `node`, else executed) or
  `klatch` on PATH. Node >= 22 is an explicit runtime dependency for the CLI
  verbs only; provisioning stays Python.
- Help, `register.sh` synopsis, embedded skill and README updated.
- Tests: `test_cli_wrapper.py` (fake CLI) plus existing `test_provision.py`
  regression. Run only on the shared test host after an advisory claim.
- Open: exact CLI interface is still settling in the Klatch owner's workspace
  (issue-14); the verb list mirrors its `commands` array today.
