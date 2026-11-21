---
id: 791513bdc319
kind: feature
title: 'Barred resources: a claim that blocks use - mark a resource as barred (e.g. ''no podman VM on the dev box'') so any attempt to use it is refused'
seq: 3292
status: todo
priority: p1
created: 2026-11-21T21:46:15.791478Z
---

Operator 2026-11-21 (after the dev-box crash: a podman VM 8.6 GB and govulncheck ~21 GB ran on the developer workstation against standing rules): 'add a story to enhance resource claim that could block the use of a resource, i.e. claim it and block the use; this resource is barred from use'. Today a claim only coordinates agents that choose to check it; nothing stops a tool from using the resource. Existing piece to build on: 'bashy resource add --guard' (argv prefix another lane enforces, stored verbatim). Shape: (1) a barred state on a resource or claim (e.g. 'bashy claim bar dev-box.podman-machine --reason "..."' or a resource flag), owned by the operator; (2) enforcement at the bashy front door and in agent-launched shells: commands matching the resource's guard argv (podman machine start/init, VM launch, govulncheck over full graphs, full go test ./..., sealed gates) are refused on that host with the reason and the override path; (3) explicit, logged override for the operator only (guards need an escape hatch); (4) bashy claim list shows barred resources distinctly; (5) seed bars for the dev box: podman/VM machines, heavy suites, govulncheck full-graph. Acceptance: with the bar set, the guarded commands exit non-zero with the reason in both an interactive bashy and an agent-launched shell; the logged override works; removing the bar restores normal behaviour. Unit tests only. Related: dev-box resource guard 15dcdf99 (thresholds); this one is an explicit policy bar.

## Operator examples 2026-11-21

The bar is per HOST + capability, with a redirect:
- (a) podman on host:X is banned (any podman/VM machine start, init, run).
- (b) go-build on host:X is banned (go build / go test / dag build targets, including those run by agents).
- Each bar may name where to go instead, e.g. "use host:elephant".

Example shape (names illustrative):

    bashy claim bar host:dragon podman   --use host:elephant --reason "no VMs on the dev box"
    bashy claim bar host:dragon go-build --use host:elephant --reason "heavy builds run on the gate host"

On host:dragon, `bashy podman machine start` or `go build ./...` (directly, through bashy dag, or from an agent shell) is refused with:
"go-build is barred on this host (reason ...); use host:elephant", plus the operator-only logged override.
