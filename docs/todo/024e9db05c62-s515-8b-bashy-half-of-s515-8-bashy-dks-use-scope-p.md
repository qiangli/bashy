---
id: 024e9db05c62
kind: feature
title: 'S515.8b bashy half of S515.8: bashy dks use @scope:<path> switches the default namespace to scope-<path>; bashy dks list shows the scope namespaces the account can reach'
seq: 2750
status: todo
priority: p1
labels:
    - dks
created: 2026-10-28T02:23:16.469751Z
assignee: codex-gpt6-sol
sprint: 515
sprint_id: e82160ea-95db-5f2a-b69e-b2d784b2996b
sprint_title: 'satellite loom: private forge + CI + groups (KISS/MVP, reuse loom/act_runner/scopes)'
---

Companion of S515.8 (cloudbox 8e6bdb0c; merged cloudbox dev 8e83b95: scopedpairing projects scopes into gated DKS namespaces scope-<path> with RoleBinding edit + quota). Spec: umbrella docs/sprint-515-satellite-loom.md (S515.8: "bashy dks context switching per namespace (use/list), reusing the kubeconfig issuance"). KISS: extend the EXISTING verbs, do not add a parallel command tree.
Existing: `bashy dks use` selects a workspace (`use <name>[@k8s:<ctx>]`, `use @ws:<id>`, `use --none`); `bashy dks kubeconfig` shows/refreshes/mints the kubeconfig (incl. --from-satellite).
Do:
- `bashy dks use @scope:<path>` (path like grade-10-9/a) sets the default namespace of the current DKS kubeconfig context to scope-<path with / -> -> > (same name rule as the cloudbox projection: scope-grade-10-9-a); `bashy dks use` shows it; switching back to a workspace keeps working.
- `bashy dks list` lists the namespaces the account can reach (scope-* via the k8s API with the issued kubeconfig), marking the current one; --json.
- Help text + the dks skill doc updated.
Red/green unit tests (name mapping, kubeconfig namespace write, list filtering) with a fake client; never exec the bashy self binary from tests. Gate: focused go test for the touched packages (engine-tag variants if the code lives behind -tags bashy_engines).
Live (later, manager): on the staging DKS satellite after the cloudbox release with S515.8.
Commit trailers: Sprint: #515 / Story: #2750 / Story-ID: 024e9db05c62.
