---
id: a52829eaaa35
kind: feature
title: 'Close reclaims everything: story close and sprint end release claims and delete run/grade copies (no resource leaks)'
seq: 3286
status: todo
priority: p0
created: 2026-11-21T11:29:39.28621Z
sprint: 652
sprint_id: 48a699fb-076f-5442-995a-b12987a45bea
sprint_title: Outpost reliability and host-offline alerting on cloudbox
---

Operator 2026-11-21 (typed): 'do it now. this should be treated a top priority it saves time/token the sooner the better. when story/sprint is closed, all its related resource should be cleared/reclaimed. or else it is a leak, right?' Agreed: anything a closed story or ended sprint still holds is a leak.

Evidence: on 2026-11-21 the conductor found 163 live story claims, 134 on done/wontfix stories or on stories no longer in any docs/todo store (sprints 382-666), held by long-gone sessions; claim prune cannot remove announce claims and claim release has no force, so cleanup needed claim --force takeover + release per item. Separately, grade checkouts leaked 10 GB of dev-host disk (todo bf2f149d, operator rule 'delete by default').

What exists (reuse, do not duplicate): yoke pkg/weave/weave_story_end_cleanup.go already reclaims run-owned artifacts at sprint end through one guarded teardown (sprintUndisposedRuns, sprintResidual, sprintReclaimSummary). It does NOT release claims and does not run on single-story close.

Scope (smallest robust change, one mechanism):
1. One reclaim function called on story close (sprint accept, todo done, wontfix, move off sprint) and on sprint end: release every story claim on the story (any holder) and every lease/resource claim whose recorded story is that story; tear down settled run workspaces and grade checkouts for it via the existing guarded teardown (never discard commits that exist nowhere else: pushed or git-cherry-proven first; an explicit --retain with owner+reason is kept until the story closes). Record each release/deletion on the room timeline with the closing verb as reason.
2. Backfill verb with dry-run default, e.g. 'bashy claim prune --stories-closed [--apply]', covering existing stale claims (closed or missing stories) and leftover grade/weave copies of closed stories.
3. Document the lifecycle in the claim and sprint skill docs (announce vs lease, who releases, close reclaims everything, --retain).

Acceptance: accepting a story with a live story claim, a lease claim naming it and a settled run workspace leaves none of them; sprint end leaves no claim or copy for its stories; backfill dry-run lists exactly the stale set and --apply clears it; a run with unpushed unique commits is refused, not deleted; targeted tests on the dev box, yoke short suite in a gate-host lane. Subsumes the claim half of the original a52829ea and the delete-by-default half of bf2f149d (link it).

### Sprint manager assignment

Use this story as the spec. Start with a design note on the story and
send it to codex-gpt6.2-sol for review. Stop there before code. Identify
the existing entrypoints, durable owner data, exact claim fence and
guarded teardown, and record how backfill can distinguish missing stories
from unrelated or unreadable stores. Preserve evidence and unique
commits. Do not run backfill against real claims or touch the gate host
during design. Sprint 677 has venue priority.

### Design Note (`agy-gemini4-carbon`, 2026-11-21)

Links & subsumes delete-by-default checkout hygiene from `todo:bf2f149daa87`.

1. **Single Reclaim Entrypoint (`ReclaimClosedStory` / `reclaimSprintEnd` in `yoke/pkg/weave/weave_story_end_cleanup.go`):**
   - Invoked from all 5 lifecycle closure paths (outside `sprint` `queue.lock` to preserve lock ordering `story/sprint -> run lifecycle lock -> weave queue.lock / claims.lock`):
     - `sprint accept` (`runSprintStoryAccept` in `pkg/weave/weave_story_claim.go`, reason `"sprint accept"`)
     - `todo done` / `todo status <id> done` (`todopkg.StoryClosed` seam wired in `pkg/weave/sprint_story_link.go`, reason `"todo done"`)
     - `todo status <id> wontfix` (`todopkg.StoryClosed` seam, reason `"todo wontfix"`)
     - `todo edit <id> --sprint <new|0>` (`todopkg.SprintChanged` seam in `pkg/weave/sprint_story_link.go` when a story moves off sprint `#N`, reason `"todo move-sprint"`, scoped to `#N`)
     - `sprint end` (`runWeaveStoryBox` in `pkg/weave/weave_story_box.go`, iterating all sprint stories + sprint-owned runs/grade dirs, reason `"sprint end"`).

2. **Durable Owner/Story Data & Claim Fence (`yoke/pkg/policy/coord`):**
   - `Claim.Story` (`json:"story,omitempty"`, format `<sprint>/<storyID>` or `<storyID>`) added to `coord.Claim` and `coord.Request` (`--story` flag on `bashy claim`, auto-populated from `WEAVE_STORY` when set by `weave start`).
   - `coord.ReclaimClaims(dir, matchFn)` acquires `claims.lock`, skips any `ModeAttached` claim whose kernel lockfile (`keyedLockPath`) is still live, and removes matching `story:<sprint>/<id>` (any holder, including `ModeAnnounce`) plus any lease/project claim whose `Claim.Story` matches the closed story via `fileBackend.commitLocked(key, cur.Rev, nil)` — compare-and-swapping `cur.Rev` and preserving the `.epoch` high-water sidecar so fencing epochs never regress.

3. **Guarded Teardown, Delete-by-Default & `--retain` (`yoke/pkg/weave`):**
   - **Weave runs:** Reuse `weavePruneOwnedRun` (`pkg/weave/weave_resource_cleanup.go`) extended with `endRetain bool`. Terminal transitions (`weave pull`, `weave abandon`, and merged/empty terminal wrapper exit) delete workspaces by default unless `--retain --retain-reason <why>` (requiring non-empty owner + reason, stored as `Retain`, `RetainOwner`, `RetainReason` on `weaveItem`) is set. Open-story `sprint prune` honors `Retain`; story close / `sprint end` sets `endRetain=true`, ending retention while still enforcing `weaveCleanupEligible` + `weaveItemSettled` (`merge-base --is-ancestor` into base or `refs/salvage/*`, `gitBranchIntegrated` / `git cherry`, `weaveUnsettledLocalRefs`, and clean `git status --porcelain --untracked-files=all`). Dirty workspaces or runs with unique unpushed/unintegrated commits are **refused and kept**, never deleted (`sprint accept` / `sprint end` refuse closure until settled or salvaged).
   - **Grade checkouts (`pkg/weave/sprint_grade.go`):** Delete `ev.Checkout` immediately after recording the `grade` event (pass, fail, tamper, or error) unless `--retain --retain-reason <why>` is passed (`Retained`, `RetainOwner`, `RetainReason` on `sprintGradeEvent`). Story close deletes retained grade checkouts for the story's runs; `sprint end` removes `$BASHY_HOME/sprint/grade/<sprint>/`.
   - **Room timeline audit:** Each reclaimed claim, run artifact, and grade checkout emits `room.Record` (`type="sprint"` or `"todo"`, `topic="reclaim"`) naming the target, former holder, and closing verb.

4. **Backfill Verb (`bashy claim prune --stories-closed [--apply] [--json]`):**
   - Dry-run default; `--apply` mutates (registered via a story-closure backfill hook wired into `coord` by `pkg/weave` so standalone `coord` stays decoupled).
   - **Distinguishing closed/missing stories from unrelated or unreadable stores:**
     - Unrelated claims (neither `Kind == "story"` nor `Claim.Story != ""`) are untouched.
     - Resolves candidate stories across sprint `StoryRoots`, linked weave repo roots, current repo/submodules, and `~/.bashy/todo/*`.
     - If a story is found and `todopkg.IsClosed(it.Status)` (`done|wontfix|closed`), OR its sprint on the host board is `done` (`!sprintColumnOpen`), it is **provably closed**.
     - If a story is absent from disk: it is **provably missing** ONLY when its sprint card exists and *every* declared `StoryRoot` on that card is a readable git checkout on this host (`<root>/.git` exists and `os.ReadDir(<root>/docs/todo)` succeeds or cleanly returns `os.ErrNotExist`). If an open sprint's root path is absent on this host or returns permission/I/O errors, the store is **unreadable/unverifiable** and backfill skips the claim/artifacts with an explicit reason rather than deleting.
     - Reclaims matching stale claims, ended/closed grade checkouts, and `weaveItemSettled`-proven run copies (keeping any dirty/unsettled workspace with reason).

5. **Docs:** Update `bashy claim` help (`pkg/policy/coord/cli.go`), `bashy/skills/bashy/commands/collaborate.md`, and `bashy/skills/bashy/commands/sprint.md` (announce vs lease, automatic close reclamation, delete-by-default, `--retain`, and `--stories-closed` backfill).

### Manager Review Safeguards & Gate Policy (`codex-gpt6.2-sol` & Operator, 2026-11-21)

1. **Live `ModeAttached` preflight refusal:** `preflightAttachedStoryClaims` refuses story close (`sprint accept`, `todo done`, `todo status ... wontfix`, `todo edit --sprint`) and `sprint end` when any live kernel-locked `ModeAttached` claim still names that story/sprint, avoiding closed-with-residue or breaking active child locks.
2. **Epoch, revision, and closure revalidation:** Live close snapshots `MaxEpochByKey` before committing and rechecks `closureStillHolds` under `claims.lock`; `--stories-closed --apply` rechecks observed `(epoch, rev)` plus fresh story/sprint status and `weavePruneOwnedRunWithOptions` under lock, preserving foreign claims, new-epoch claims, and claims on concurrently moved/reopened stories.
3. **Authoritative declared store for missing stories:** Missing-story backfill requires a sprint card on this host with at least one declared `StoryRoot` where every declared root is an initialized, readable git checkout and the host todo store is readable; bare `--story` tags without a sprint card are never assumed deleted.
4. **Test-fixture root-cause log (initial test run):** In `story_reclaim_test.go` (test defect, fixture setup order only): two fixtures stamped `status=done` before calling `sprint link --story` (which refuses linking already-closed stories), and one booth grade fixture omitted a terminal `State` on the mock `weaveItem` before testing `wontfix` grade cleanup. Fixed in `story_reclaim_test.go` by linking while open before simulating pre-existing `done` state and marking the mock booth item terminal before `wontfix`.
5. **Verification & Deferred Milestone Gates:**
   - **Executed per story (fast dev-box unit tests + 3-OS build):** `pkg/policy/coord`, `pkg/prune/...`, `pkg/todo/...`, and targeted `pkg/weave` reclaim/grade/claim/cleanup unit tests + `darwin`/`linux`/`windows` builds (`CGO_ENABLED=0`). No live host backfill `--apply` executed during implementation.
   - **Deferred to `.bashy/gate.final` (milestone integration gate per Operator policy):** Full `yoke` suite (`go test -short ./...`), full `bashy` integration + Bash 5.3 `86/86` gate, gate-host lane execution, and browser/judge/e2e suites — deferred to final gate.

### Pause / Stop Checkpoint (Operator Directive, 2026-11-21)

Paused per Operator instruction ("pause all non-677 work until 677 completes; commit and push on branch, no merge, release claims"):
- **Where stopped:** Implementation, manager review safeguards, skill docs, and targeted fast dev-box unit tests + 3-OS builds are complete and committed on isolated branches (no merge to `main`, no `go.mod` or umbrella pin bumps):
  - `yoke` branch `agent/weave-issue-96` (`0dafbbff82df`, workspace `yoke#96` preserved in place)
  - `bashy` branch `agent/weave-issue-128` (workspace `bashy#128` preserved in place)
- **Remaining when resumed after Sprint 677:** Rebase `agent/weave-issue-96` and `agent/weave-issue-128` onto post-677 `origin/main` heads (or replay small patch onto fresh heads if rebase is nontrivial), rerun fast unit tests + build-only `.bashy/gate`, bump `bashy` -> `yoke` pin on merge, and close via manager review.
- **Claims:** Sprint 652 story claim yielded/released; no resource or project claims held.

### Post-677 Resume & Standalone Adapter Coverage (`agy-gemini4-carbon`, 2026-11-23)

1. **Current-head producer & consumer branches (zero conflict on current `origin/main` heads):**
   - `yoke#100` (`agent/weave-issue-100` @ `20c36534`, based on `yoke` `origin/main` `4bafb600`): full close-reclamation implementation + manager safeguards (`pkg/policy/coord`, `pkg/todo`, `pkg/weave`), clean tree, fast `GOWORK=off` unit suites (`coord`, `prune/...`, `todo`, `weave` reclaim/grade/claim/cleanup) + `darwin`/`linux`/`windows` `CGO_ENABLED=0` builds green.
   - `bashy#138` (`agent/weave-issue-138`, based on `bashy` `origin/main` `847f7032`): skill docs (`collaborate.md`, `sprint.md`) + post-677 standalone adapter wiring (`yoke/todo/main.go` blank-imports `github.com/qiangli/yoke/pkg/weave` so standalone `bashy todo` / `bashy issue` wires `SprintHandles`, `SprintChanged`, `SprintPreflight`, `StoryClosePreflight`, `StoryClosed`, `StorySprintMovePreflight`, and `StorySprintMoved`; verified with `AssertStandaloneDeps` in `yoke/todo/todo_test.go`).
2. **Manager integration note (dual `go.mod` pin bump required after Sprint 677):**
   - Merge/push `yoke#100` (`20c36534`) to `yoke` `origin/main` first, then bump `github.com/qiangli/yoke` in **both** root `bashy/go.mod` (in-process `claim`) **and** `bashy/yoke/go.mod` (used by standalone `GOWORK=off` adapter builds for `yoke/sprint`, `yoke/weave`, and `yoke/todo`), then pull `bashy#138`.
