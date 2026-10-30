---
id: d440c5f5a85c
kind: task
title: S541.b bashy claim who PATH + agent --json cwd
seq: 2960
status: done
created: 2026-10-30T17:41:36.843212Z
weave: 360
assignee: claude-opus5
sprint: 541
sprint_id: 3013eae1-4380-53b7-a528-e53732910595
sprint_title: 'Proactive agent collaboration: collaborate skill, worker preamble, claim who'
closed: 2026-10-30T19:07:40.634282Z
closed_by: s541-manager
---

Repo: bashy only. Spec: docs/sprint-541-master-execution-plan.md (dhnt umbrella).

DELIVER `bashy claim who [PATH...] [--json]` as a subcommand added to the coord claim command where bashy wires it (bashy/internal/agentos/agentos.go case "claim": coord.NewClaimCmd(...) -> cmd.AddCommand(newClaimWhoCmd())), implemented in a new bashy/internal/agentos/claim_who.go. Compose existing data only, no new store:
 - coord ledger claims whose Roots/Members contain the path (yoke/pkg/policy/coord exported list API);
 - live room cards (yoke/pkg/room) whose Cwd contains the path (deepest match first);
 - weave holders: yoke/pkg/weave/guard.go HoldersOf(repoRoot, HoldersQuery{}) for the path's repo;
 - for committed-but-unpushed commits touching the path (@{u}..HEAD), the Agent: / Story-ID: / Sprint: trailers.
Output per path: holder name, source (claim/room/weave/commit), intent/task, pid + liveness, age, and a ready-to-run line `bashy mb send <holder> "..."`. No PATH => the dirty paths of the current checkout (git status --porcelain via the existing bashy git/go-git helpers). Unknown owner is reported honestly ("no owner found; post mb --topic announce"), never guessed. --json emits a stable envelope.
ALSO add a `cwd` field to agentAssignment (bashy/internal/agentos/agents.go:115) populated from the room card Cwd, in --json (table unchanged).
VERIFY: red/green unit tests first, one per source (claim, room cwd, weave holder, commit trailer, none) using temp BASHY_HOME/room/weave state dirs; never exec the bashy self binary in tests. `go test ./internal/agentos -run 'ClaimWho|AgentRoster'` green. No other repos.
