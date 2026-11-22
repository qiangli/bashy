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
- Each bar may name where to go instead, e.g. "use host:<gate-host>".

Example shape (names illustrative):

    bashy claim bar host:<dev-box> podman   --use host:<gate-host> --reason "no VMs on the dev box"
    bashy claim bar host:<dev-box> go-build --use host:<gate-host> --reason "heavy builds run on the gate host"

On host:<dev-box>, `bashy podman machine start` or `go build ./...` (directly, through bashy dag, or from an agent shell) is refused with:
"go-build is barred on this host (reason ...); use host:<gate-host>", plus the operator-only logged override.

## Design (operator-approved 2026-11-21): one gating tool, same design as @guard

Operator: "it should be same design as the @guard so it is applicable not only to the top-level bash commands; it could be applied in bash# scripts and subscripts as one consistent gating tool."

1. Rules = match -> action.
   - Match facts: host (role/name/OS/free mem/disk), command/capability (atlas group, effects, caps such as podman, go-build), kind of work (tags declared on dag targets and skill-scripts: unit, integration, e2e, cert, parallel), who (agent, sprint, operator).
   - Actions: allow | deny (barred, with reason) | route (host:<gate-host>, droplet:posix-cert, aws:e2e-fleet) | queue (until resources free).
2. Declarative rules first; Bash# predicates only for what rules cannot express, marked `@guard read` (pure, read-only, fast). Example policy skill-script:

       # policy/venues.bsh - first match wins
       rule cert        --kind cert        --route droplet:posix-cert
       rule e2e-par     --kind e2e --parallel --route aws:e2e-fleet
       rule integration --kind integration --route host:<gate-host>
       rule no-vm       --host the dev box --cap podman --deny "no VMs on the dev box" --use host:<gate-host>
       rule unit        --kind unit        --allow
       @guard read
       fn heavy(ctx) { ctx.cap == "go-build" && ctx.host.role == "dev" && ctx.host.mem_free_gb < 4 }
       rule low-mem --when heavy --queue

3. ONE engine, enforced at every exec boundary, not just the front door: the bashy front door, the in-shell ExecHandler ring (every command a script runs, including subshells, sub-scripts and command substitutions), dag target bodies (alongside their Effects caps) and skill-scripts. The same rule therefore gates `go build` typed by a human, run by an agent, or run three scripts deep.
4. Composes with @guard: host policy is the outer bound; a script's or function's @guard can only NARROW it, never widen it. A denial names the matching rule (policy file + rule name), the reason and the route.
5. Policy lives in one versioned place: public rules in the umbrella; private host names in a local, git-ignored file. `bashy policy check <cmd>` explains which rule matched and why. `bashy claim bar host:X CAP --use host:Y` is shorthand that writes a rule.
6. Operator-only logged override (guards need an escape hatch).

Acceptance (unit tests): the same rule denies/routes a command at the front door, inside a Bash# script, inside a sub-script it calls, and in a dag target body; a @guard cannot widen a host denial; `bashy policy check` reports the matching rule; the override is logged.
