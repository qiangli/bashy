---
id: fd0f84a041f3
kind: test
title: POSIX differential harnesses must distinguish unavailable oracle from shell mismatches
seq: 341
status: assigned
priority: p1
labels:
    - posix
    - macos
    - harness
created: 2026-09-24T17:16:42.006557Z
assignee: codex-gpt5.6-sol
sprint: 274
sprint_id: e14729b9-4d75-5803-aca0-594e943ca2df
sprint_title: Bash# POSIX and full Go test parity on Windows and macOS
---

On novidesign.local, Podman oracle startup failed with an SSH handshake EOF. austin-defects.sh then reported 37 false shell diffs because it parsed empty oracle output after OCI failed. posix-parity-pty.sh likewise reported six ERROR rows after the oracle process failed before prompt. Add oracle invocation/preflight checks so infrastructure failures are blocked and never counted as semantic deltas. Validate with a forced OCI failure and rerun on macOS when host storage permits. No Bash# implementation behavior changes.
