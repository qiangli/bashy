---
name: account
description: Host-local account inventory — list/show/add/set/remove/restore accounts, discover and refresh from providers, usage, resources, resolve, and a JSON schema.
---

# account

`account` keeps a host-local inventory of provider accounts. Cached reads (`list`, `show`, `schema`) touch only the local store; provider calls are confined to `discover`, `refresh`, `usage` and `resources`.

```
account list [--category C] [--provider P] [--team T] [--status S] [--json] [--fields a,b]
account show ID --json
account add / set / remove / restore ID
account discover | refresh | usage | resources
account resolve REF
account schema --json
```

Set `BASHY_HOME` to relocate the store. `--json` emits the versioned envelope; errors are also JSON on stderr.

See `bashy account --help` and `bashy help common-options`.
