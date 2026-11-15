---
id: 9b8ce84efc41
kind: bug
title: bashy satellite rm refuses --json ('--json unsupported for satellite') although the yoke verb has the flag
seq: 3239
status: todo
priority: p2
labels:
    - fleet
created: 2026-11-14T22:43:01.718786Z
---

Seen 2026-11-14 in the cloudbox S23 fleet suite user-complex with a bashy built from the umbrella siblings (bashy 299b611e-dirty): 'bashy satellite rm <id> --yes --master URL --json' exits with 'bashy: --json unsupported for satellite' while 'bashy satellite transfer ... --json' and 'bashy satellite admit set ... --json' work in the same run. yoke pkg/fleet/satellite_rm.go declares --json (S12, tested in yoke). Likely the bashy front door's json-capability table does not list the rm subcommand. The fleet suite calls rm without --json meanwhile.
