---
id: 3257b40fcc9a
kind: bug
title: bare conform in a bashy shell is bashy's own test batteries and shadows the dhnt/conform judge binary on PATH
seq: 3323
status: todo
priority: p2
labels:
    - conform
created: 2026-11-26T09:41:44.269666Z
---

Found in the Sprint 723 arm probe (2026-11-26). Shapeware spec section 19 says the conform suite ships with bashy and agents use it while building; a task that names conform run CONTRACT.md reaches bashy's compat/compliance/benchmark verb instead when the agent's shell is bashy (unknown command for run/version). The judge is reachable only as bashy conform. Decide one name: route the judge's subcommands (run, cert, mutate, spec, version) through the bare verb, or rename the batteries verb. Workaround used in the drill: a plain system shell in every arm.

Correction (2026-11-26, same day). Current bashy source already routes `conform run|cert|mutate|version` to the dhnt/conform judge in-process (internal/agentos/verify.go); the dev box's installed build predates that, which is why the arm saw "unknown command". So the judge is reachable in a current bashy shell. What remains:

- One verb carries two unrelated things: bashy's own batteries (compat, conformance, compliance, benchmark) and the judge. Its help text opens with the batteries, and `conform --list` and bare `conform` list only them.
- `conform spec check` (dhnt/conform, Sprint 723 S7) is not routed.
- Proposed split (operator to confirm the name): `conform` = the judge only, argv handed verbatim to dhnt/conform so it equals the standalone binary; the batteries move to their own verb; `conform compat|conformance|compliance|benchmark` keep forwarding for one release with a one-line stderr notice. Only three references in the repo use the batteries through `conform`.
