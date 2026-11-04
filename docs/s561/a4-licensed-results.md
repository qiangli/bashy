# Sprint 561 A4: licensed run results (counts only)

Licensed journals and reports stay outside Git, in the operator's private
evidence store. This file records identities and aggregate counts only.

Approval (separate layout): approval `309645b7`, harness `0b2b185d`, Bashy
`f1bcd711`, sh `74586fde`, coreutils `211789fa`, public umbrella `4e6c63ed`;
transfer validator PASS.

## Profile B — 493-TP shell release gate (2026-11-04)

Arm `shell-release-20261104-s561-r1`, disposable leased host (retired after
verified retrieval). Result: **PASS** — 1 set, 493 declared / 493 observed
TPs, 493 in the certification PASS group; missing 0, caps 0, runner failures
0, blockers 0, manual resolutions 0.

## Profile D — full Sprint-341 base run

Running. Arm `profile-d-s561-20261104` launched 2026-11-04 on a leased
disposable host after readiness PASS; clean-host gate PASS: 102 commands =
92 Go + 10 shell, 0 external providers, 14 optional excluded, 0 GNU
reference prefixes. Durable systemd unit, 9 h process ceiling. Retrieve with
the kit's Profile D lifecycle (`profile-d-do.sh status` / retrieval), then
retire the host with `do-retire-ephemeral.sh`.
