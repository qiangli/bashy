# Hosted runner compiler evidence repair — Sprint 436, Story 1927

Preserve the interrupted worker commit and finish its compiler-time content store.
Resolve dependencies in each invocation cwd, retain content-addressed bytes and
full argv/cwd, narrowly admit private Go-generated files and runtime/cgo sources,
and reject unknown external native inputs. Audit captured paths portably after
Go removes temporary files. Keep exact repository C/C++/Metal license admission.

Discover installed Windows GCC/G++ pairs, verify native target and language
identity, record their paths/version, and use the selected paths for the build.
Upload diagnostic evidence with always(), retaining all build/audit/release gates.

Validate with bounded Python fake-compiler/adversarial fixtures, workflow checks,
and the recipe regeneration gate. No native compilation or runner on dev-host.
Hosted five-target execution and exact license clearance remain manager work.

## Delivery evidence

The completed branch retains preserved worker commit f47940c4 and its parent
980cd040. Bounded `bash builds/check.sh ollama` passes: 4 recipe tests (including
all five fake target builds and 12 concurrent captures), 14 native protocol
contract fixtures, 7 compiler-evidence adversarial tests, 5 audit policy tests,
11 shared egress cases and 19 shared toolchain cases. Generated fork reproduction,
shell syntax, installed shellcheck/actionlint checks and `git diff --check` pass.
A `bashy gate --command 'bash builds/check.sh ollama' --json` invocation also
reported passed. The final gate transcript is `/tmp/bashy-issue-101-gate.log`.

No native compilation, inference, network-confinement proof, license clearance,
push, branch switch or hosted workflow dispatch was performed. The manager must
integrate this branch descendant (including the preserved WIP), install/render
the producer branch, tag the next immutable revision, and inspect hosted results.
The exact native license review map remains unchanged and fail closed.
