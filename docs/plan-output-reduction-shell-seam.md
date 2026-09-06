# Sprint 123 shell output-reduction seam and Stage 1 activation

Scope: predecessor Story #66 (`f5a2f2ca2b19`) and activation Story #284
(`3474a9acc844`).

1. Compose the Bashy runner's stdout/stderr once in `agentos.WireExec`, above
   the POSIX return, while leaving the standalone `cmd/bash` hook a plain
   `interp.StdIO` installer.
2. Capture only the runner-level, agent-facing sink. Flush around dispatched
   external/in-process commands, preserving pipes, redirects, command
   substitution, non-zero evidence, and the secrets render path.
3. Apply the coreutils content-addressed reducer and conservative atlas shape:
   result output is capped at 40 KiB; successful verdict output becomes one
   recoverable line; failures remain complete after the same redaction gate.
4. Mount `bashy out` on the existing Bashy session artifact tree. Stage 1 is
   explicit opt-in only: `--reduce`, `BASHY_OUTPUT_REDUCE=on|true|1|yes`, or
   the scoped `BASHY_OUTPUT_REDUCE=profile:<name>` form. `BASHY_AGENTIC` alone
   never activates reduction. `--no-elide`, `--full`,
   `BASHY_OUTPUT_REDUCE=off`, and `bashy full --` are escape hatches; `off`
   wins even when `--reduce` is also present.
5. Prove activation, POSIX/certification behavior, dry-run composition,
   external/coreutils streams, recovery/determinism, sink fidelity, warm
   returned writers, PTY delivery, and the `cmd/bash` import boundary with
   focused tests, then run the repository gate without Docker/sandbox lanes.
