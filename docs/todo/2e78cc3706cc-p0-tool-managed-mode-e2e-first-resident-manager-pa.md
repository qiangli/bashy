---
id: 2e78cc3706cc
kind: task
title: 'P0: Tool-managed mode E2E first; resident-manager parity'
seq: 234
status: todo
priority: p0
created: 2026-09-06T02:51:32.68482Z
sprint: 130
---

PURPOSE

Make the shared bashy sprint command substrate and the TWO supported sprint-manager modes explicit, implemented, and covered end to end. A sprint is the durable cross-tool session: its board, dedicated Meet room, thread, continuity brief, and linked todos preserve the work and context that a single agentic tool normally keeps in session history and private memory files. Transfer between vendors must feel like rename/export followed by init/resume, without impersonating the previous holder.

SPRINT 127 EXECUTION SCOPE

This story adds tests and fixes confirmed defects in behavior the product already claims to support. A failed assertion that proves an existing path is broken produces an active bug. A missing capability or new entry path not already supported produces a deferred feature story; it is documented but not implemented in Sprint 127. Do not make a feature must-pass merely by naming it in this test plan.

SHARED BASE — RAW SPRINT COMMANDS, NOT A THIRD MODE

The raw sprint command surface is the foundation used by both manager modes. A sprint may be ownerless while it is an inactive draft/backlog record, or inside an isolated unit test that exercises only record behavior. In production, entering an open/started state MUST require a real agent already registered in `bashy agents`; no missing, placeholder, synthetic, or dummy owner may satisfy that invariant. Tests of an owned workflow must register a real fixture agent through the same public path production uses.

A human, agent, or automation can use the base commands to create, edit/rename, move, inspect, and prepare an inactive sprint. Once a registered agent owns it, the same surface drives focus, checkpoint, stop, restart, handoff, and end. The board, dedicated Meet room, thread/context, goal checklist, continuity state, and linked todo stories remain durable across those transitions. The base must neither invent an owner nor provide a test-only bypass around owner validation.

MODE 1 — EXTERNAL AGENTIC TOOL IS THE MANAGER

Claude, Codex, OpenCode, agy, or another external CLI registers or reuses its own unique canonical bashy agent name and becomes the sprint owner/manager under that exact name. It must never inherit or impersonate the prior vendor identity.

The external manager:
  * starts a dedicated subagent or equivalent independently-running watcher for its own bashy inbox, keeps it reading, and acknowledges handled batches;
  * runs the conductor autopilot loop: mail first; re-read the dynamic story board; reprioritize before staffing; assign independent stories based on live availability, capacity, and capability; monitor progress rather than mere liveness; respond to users and agents; review and merge sequentially; run gates itself; clean only sprint-owned workspaces; commit, push, and bump umbrella pins when repository policy requires them; checkpoint; repeat;
  * notices stories added while the sprint is running and incorporates them on the next tick without restart;
  * stops and hands off with an actionable written brief that a different vendor can resume using only the one-sentence takeover instruction.

MODE 2 — DEDICATED AUTONOMOUS BASHY AGENT IS THE MANAGER

A registered agent selected from bashy agents holds the sprint seat and follows the same owner identity, inbox, dynamic-priority, delegation, monitoring, integration, gate, cleanup, delivery, response, and continuity rules as Mode 1. It can be initiated through all supported entry paths:
  a) by an external agentic tool;
  b) by a human from the bashy sprint CLI;
  c) by a human from bashy apps or the Sprint/Meet web UI.
The managed agent survives detachment and remains steerable through the sprint dedicated Meet room and durable inbox.

HUMAN STEERING IS REQUIRED, NOT OPTIONAL. A human using Bashy Apps must be able to steer the active sprint manager through both the Messages (`mb`) and dedicated Meet-room surfaces. Both paths must address the registered manager identity and land in that manager's ONE durable unified bashy inbox, where its watcher can read and act on them. The web UI must reuse the same MB/Meet delivery paths as the CLI; it must not create a browser-only message channel or treat a successful send response as proof of receipt.

RECONCILIATION

The current usage record and script/e2e-sprint-modes.sh use the older labels Mode 1 external manager and Mode 2a/2b managed manager. Reconcile the documentation, help, conductor skill, and tests around one shared raw-command base plus these two manager modes. Preserve the useful transport distinction: attached external manager versus bashy-managed detached manager. Do not create a new store, transport, identity system, board, room, or lifecycle noun.

E2E GATE

Extend script/e2e-sprint-modes.sh or its canonical successor so the shared base and every supported manager behavior above have explicit deterministic assertions. At minimum prove:

  1. the shared base supports ownerless inactive record operations, but production start/open refuses a missing, dummy, placeholder, or unregistered owner; after registering a fixture agent through `bashy agents`, start/checkpoint/stop/restart/handoff/end work and retain board, story, thread, goal, Meet-room identity, and continuity state;
  2. an external manager must use its own registered unique name, can hold the seat through its independently-running inbox watcher, receives newly-added dynamic work, reprioritizes it, delegates it, and produces an attributable handoff;
  3. a managed bashy agent can be started through CLI/external-tool and web entry paths, runs detached, is reachable and steerable, and preserves the same seat address and room across restart/handoff; steering instructions sent through sprint ping, the Bashy Apps Messages/MB path, and the sprint's dedicated Bashy Apps Meet-room path must each be read back with their bodies intact from that registered manager agent's unified bashy inbox, proving delivery rather than trusting any send command or HTTP response;
  4. both manager modes demonstrate the full tick loop through assignment, notification, run observation, independent gate verdict, integration/cleanup evidence, checkpoint, and response;
  5. rename/export-style handoff followed by init/resume-style takeover preserves the durable sprint context while changing the holder identity;
  6. existing supported behavior is must-pass, not silently skipped. A broken supported path is an active Sprint 127 bug and must become green. A genuinely new or unsupported capability is an explicit PLANNED ratchet with a linked deferred feature story and does not authorize implementation in this sprint;
  7. run live one-sentence takeover rounds on at least two external tools from different vendors in addition to the hermetic gate. Record the tool, identity, first correction if any, and resulting defect in the Sprint 127 thread.

DONE

The shared base and both already-supported manager modes are documented consistently; production cannot open a sprint without a real registered agent owner; tests contain no dummy-owner bypass; every supported entry path is represented; every supported-path assertion in the hermetic E2E gate is green; unsupported capabilities are clearly marked PLANNED and linked to deferred feature stories; two different-vendor zero-correction takeover rounds are recorded; dynamic story addition is proven; and the operator confirms the workflow is good enough. A green process exit without these artifacts is not evidence.
