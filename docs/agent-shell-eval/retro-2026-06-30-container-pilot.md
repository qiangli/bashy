# Retro: 2026-06-30 Container Pilot

## What Worked

- The container harness now compares `bashy v0.4.0` against real GNU Bash 5.3
  under `bashy podman`.
- Per-run summaries were written in `docs/agent-shell-eval/test-*.md`.
- The run captured time, pass/fail, shell invocations, tool calls where
  available, retry count, retry sleep, and API/timeout signals.
- The conductor loop caught and fixed harness defects before treating results as
  product evidence.

## What Did Not Work

- Running the agent CLI inside the container was not practical because the local
  agent binaries are host-native. The pilot used host-agent orchestration with
  container-enforced shell execution.
- The first result JSONL format was too permissive; embedding large nested stream
  JSON broke valid JSONL on Claude rows.
- Claude and agy adapters needed CLI-specific handling before valid runs.
- `agy` can produce very long stalls or timeout/retry cycles. The harness records
  this, but future runs need a clearer per-tool timeout budget.

## Bashy Improvements

- Implemented `-l` / `-lc` handling so common Bash login-command invocations work.
- Implemented `--dry-run` as an alias for `--dryrun`.
- Remaining: make dry-run more discoverable in the front-door help surface.
  Agents still had to probe heavily to find the correct spelling and behavior.

## Bashy Tool Retro

### What Worked Well

- `bashy podman` was good enough to serve as the enforcement layer for the
  comparison. The harness used normal Podman-style `run --rm -v ... -w ...`
  behavior, so the shell arm under test was isolated in a container while the
  agent client stayed on the host.
- The host build with podman support made the evaluation practical without
  requiring a separate runtime setup. Once the images existed, repeated task
  runs were straightforward.
- `bashy` as the container entrypoint made it easy to force all task commands
  through the bashy shell arm. The GNU arm used the same container shape with a
  GNU Bash entrypoint, which kept the comparison clean.
- The existing dry-run feature was real enough to help: once discovered, `agy`
  did use `--dryrun scripts/cleanup.sh` before running the destructive cleanup
  task for real.
- Bash compatibility bugs surfaced quickly under agent behavior. The missing
  `-lc` path was exactly the kind of drop-in incompatibility agents will hit in
  real use, and the fix was small and local.
- The shell wrapper plus `bashy podman` command log gave a clear audit trail of
  what each agent actually executed. That made it possible to separate task
  success from harness success.

### What Should Be Improved

- Dry-run discoverability is still too weak. Agents naturally tried
  `--dry-run`, `bashy --dry-run`, `set -o dryrun`, `shopt`, `help`, and binary
  string searches before reaching the implemented `--dryrun` path. The alias is
  now implemented, but front-door help still needs to advertise both spellings
  and a short example.
- `bashy --help` / `bashy -h` did not provide useful shell-extension guidance in
  the pilot. For agents, help output is a product surface. It should include
  agentic shell options such as `--dryrun` / `--dry-run`, `BASHY_AGENTIC=1`, and
  the expected JSON-lines manifest shape.
- `bashy podman` emitted a host resource probe warning when the harness narrowed
  `PATH` and `sysctl` was not found. The run still worked, but tool output noise
  can distract agents. Resource probing should use robust absolute-path lookup
  or degrade silently unless diagnostics are requested.
- The dry-run surface is powerful but not obvious from inside a container. Agents
  looked for docs and help inside the image and found only the binary. Consider
  shipping a compact `/usr/local/share/bashy/help` or `bashy help dryrun` text in
  the worker image.
- The dry-run option spelling was inconsistent with agent expectations. Keeping
  `--dryrun` is fine, but `--dry-run` should be treated as first-class in docs,
  help, examples, and tests.
- The current dry-run result is human-readable by default and JSON-lines only
  under `BASHY_AGENTIC=1`. Agents may not infer that environment variable. The
  CLI should make the machine-readable mode easy to discover, perhaps via
  `bashy --dry-run=json` or `bashy --dryrun --json` if that fits the flag model.
- The container harness exposed an entrypoint sharp edge: because the image
  entrypoint is already the shell, passing `/bin/bash /workspace/.verify.sh`
  made bashy try to execute the binary as a script. This was a harness bug, but
  it suggests examples should be explicit about how to invoke shell-entrypoint
  images.
- `bashy` can win agent time only if the agent can see and trust its affordances
  early. Hidden features do not help an agent; they create exploration cost.

### Product Follow-Ups

- Add tests for `--dry-run` alias parity with `--dryrun`.
- Add a help test that verifies the bashy-only extension help mentions dry-run.
- Add `bashy help dryrun` or equivalent focused help.
- Add a small `bashy doctor agent` or `bashy commands --agentic` path that prints
  the features agents should consider before running risky shell commands.
- Make `bashy podman` resource probing resilient when `PATH` is minimal.
- Add a benchmark task that explicitly scores whether agents discover and use
  `BASHY_AGENTIC=1 bashy --dryrun` JSON output.

## Conductor Improvements

- For larger implementation sprints, use `bashy weave` as the default execution
  model: decompose work into disjoint issues, assign fleet members to isolated
  workspaces, review their diffs, and merge accepted changes back into the main
  repo. The conductor remains responsible for integration quality.
- Keep separate classes for `valid`, `invalid-harness`, and `tool-adapter-fail`.
  The first raw result file mixed these too loosely.
- Emit curated results after every matrix, not only raw JSONL.
- Keep retry and timeout behavior as first-class metrics; do not hide it as
  noise.
- Sanitize docs before finishing so local paths, hostnames, and usernames do not
  enter repo documentation.

## Next Sprint Recommendations

- Add three harder tasks:
  - dependency/tool availability diagnosis;
  - destructive-script dry-run with manifest parsing;
  - wrong-host or wrong-CWD recovery with misleading logs.
- Add repeated trials per task, at least 3 per tool/shell arm, before claiming a
  statistically meaningful advantage.
- Add a small parser that extracts token/cost summaries from each tool into
  stable fields instead of embedding raw event payloads.
- Consider an optional Linux-agent image path later, but only if auth and install
  costs are understood. The current host-agent/container-shell model is good
  enough for pilot evidence.
