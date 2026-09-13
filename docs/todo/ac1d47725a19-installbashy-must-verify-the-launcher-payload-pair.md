---
id: ac1d47725a19
kind: task
title: installbashy must verify the launcher/payload pair, not just behavior
seq: 245
status: todo
priority: p1
created: 2026-09-06T11:33:40.786344Z
sprint: 130
---

tools/installbashy refuses "a binary without the required AgentOS command
surface", but that check cannot see the defect that actually occurred.

verifyBashySurface (main.go:167) probes BEHAVIOR:

    run(exe, "-c", "-l", "echo ok")
    run(exe, "commands", "--json")

The Go payload passes both. So when `dag build` produced no native launcher,
installbashy installed the payload as ~/.local/bin/bashy, ran its own
verification against it, and printed "installed ... (dhnt user bin)". Every
probe was green and the install was wrong.

The file states the contract five lines above the bug: "Unix builds are a
native pre-Go signal launcher plus a sibling Go payload. Install the payload
first so replacing the launcher can never expose a path whose companion is
missing." The .real copy loop is already conditional on os.Stat succeeding —
a MISSING sidecar is treated as "nothing to do" rather than as a violated
contract.

FIX: on linux/darwin, require the pair. If <src>.real does not exist, fail
naming the platform and the expected path instead of silently installing a
single file. Cheap version is one os.Stat; better is to also assert the two
differ (a launcher that IS the payload is the exact observed failure — both
were byte-identical to their .real sidecars).

WHY IT MATTERS BEYOND THIS BUG: nothing else stands between a bad build and a
host install. The user runs one command and reads one success line. This is the
fail-closed half of the make/dag parity story (63d6a8c6): the gate catches the
divergence at authoring time, this catches ANY bad pair at install time,
including one produced by a path neither recipe owns.

SIGNALS ARE THE STAKE: what the launcher provides is pre-Go signal handling,
and its absence shows only under signals — which on darwin is exactly where
this stack has been bitten before (a dup2 over a fixed fd stealing the Go
runtime's signal pipe). A silent downgrade here is not cosmetic.
