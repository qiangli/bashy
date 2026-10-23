---
id: b5822f4fba00
kind: feature
title: 'M2b (bashy) local-PR surfaces: mount src M2 subcommands, approve writes protected approvals, doctor + fleet registry provenance, dispatcher runs the local build'
seq: 1942
status: done
priority: p0
created: 2026-10-23T16:54:47.982148Z
weave: 273
assignee: muse-conductor
sprint: 458
sprint_id: 3b16e60f-dce3-5dd1-8ed1-4384110d0741
sprint_title: 'bashy is all you need, MVP: invariant core + every other command as editable Bash# skill/script'
closed: 2026-10-23T18:03:36.625612Z
resolution: fixed
closed_by: muse-conductor-m
---

M2b (bashy half of M2). Starts after M1b (44ee7d2a) merges, in the same repo. Engine: yoke pkg/src at yoke 44a44a7e (M1 + M2): branches, PRs, patches, ApprovalRecord, State(repo), Upgrade. Spec: umbrella docs/bashy-invariant-core.md sections 2, 3 and 6. Repo: bashy ONLY. Public: no hostnames, user ids or cloudbox internals.

Deliver:
1. `go get github.com/qiangli/yoke@44a44a7e`; `go build ./...` green.
2. Mount the M2 subcommands under `bashy src` (branch, status, diff, log, pr, patch, upgrade) and update the cobra schema, atlas and commands description.
3. `bashy approve` gets a protected-edit approval form that writes the pkg/src ApprovalRecord only after a recorded human answer through the existing ask/approve human channel. An agent cannot self-approve: refuse when no human answer is recorded.
4. Provenance: `bashy inspect doctor` shows each fetched repo's State (pristine / modified / modified-protected, base release, local HEAD); the fleet registry `kind: tool` record for bashy carries the same fields. Never report pristine without checking.
5. Run local: for an edited skill-tier Bash# unit under $BASHY_HOME/src, the dispatcher runs the edited source (interpreted). For a Go change, `bashy self build` from `local` into a cache; the dispatcher prefers the cached build only when its source differs from the release, and never for a build that touches core paths. A modified-protected build is never started as a service. A revert (local == pristine) restores the release binary.

Acceptance, red/green: approve refuses without a human answer; doctor shows modified after a fixture PR merge and pristine after revert; the registry record fields; the dispatcher picks the edited Bash# unit, and the release binary after revert; a core-touching build is refused. Focused go test on touched packages + `go build ./...`. Live on the dev box: edit one skill-tier unit, PR, merge, run, doctor shows modified, revert, doctor shows pristine.
