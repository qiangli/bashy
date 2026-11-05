# Sprint 564 master plan: software shaping for Bashy apps

Sprint UUID: `0f09b646-9acd-5bec-849d-2cf3b0e4edef`. Planning only: this doc
rectifies the card and stories, and authorizes no implementation. The card and
its stories are authoritative.

## Origin

- **Sprint 526** shipped Software Shaping for web apps (`shaping-trial.md`).
  The user describes a change in plain words at `/shape/<app>/`, genie writes a
  candidate, the trusted verifier checks it, and the user reviews and approves
  it. The result is visible only to that user until shared.
- **Sprint 563 finding F14** recorded a manual shaping of Klatch, a service app,
  on the Windows host: host git, two podman builds, an image swap and a secret
  copy (commits `469031ce`, `0fe385ad` on the Klatch shaping branch).
- **The S562 handoff retro** (`retro-s562-manager-handoff.md`) showed the same
  need for bashy itself: a feature such as live handoff (Sprint 570) should be
  shapeable and usable in real time, not only filed.

## One model, two front-ends

Implementation contract: [Software Shaping contract](shape-contract.md) (H1).

Shaping is one feature. A principal holds a version history of an app. Each
version is produced by a human, by genie, or by a human assisted by genie. The
trusted verifier checks it. It is activated for its scope, shared through
offer/adopt, and upstreamed into the sprint flow.

- **Web front-end (existing):** the `/shape/<app>/` page and `/api/shape/{app}`
  (`yoke/pkg/webconsole/shaped_candidate.go`).
- **CLI front-end (new):** `bashy shape`, mirroring the API verb for verb.
- **Engine (existing, shared):** `yoke/pkg/shape`, with packages `state`,
  `gate`, `verify`, `evidence`, `contain`, `boundary`, `appserver` and
  `selfbin`. The CLI calls the engine directly, not over HTTP. An optional
  `--remote <console>` drives another host's console through the same API.
- **Plumbing (existing):** `bashy src` (fetch, status, check, preview, history,
  name, restore, undo, release, offer/adopt/decline, pr). It stays the low-level
  layer for scripts and power users. `bashy shape` is the porcelain.

### CLI ↔ API mapping

| Web API (existing) | CLI (new) | Notes |
| --- | --- | --- |
| `GET /api/shape/{app}` | `bashy shape status <app>` | active version, candidates, scope |
| `POST …/candidates` | `bashy shape wish <app> "…"` | genie candidate from plain words; `--from <workspace>` turns a hand-made change into a candidate |
| `GET …/candidates/{id}` | `bashy shape show <app> <id>` | |
| `GET …/candidates/{id}/diff` | `bashy shape diff <app> <id>` | |
| `POST …/candidates/{id}/verify` | `bashy shape verify <app> <id>` | same trusted verifier; each app kind plugs in its own checks |
| `POST …/candidates/{id}/approve` | `bashy shape approve <app> <id>` | activates in the kind's scope |
| `POST …/candidates/{id}/reject` | `bashy shape reject <app> <id>` | leaves nothing behind |
| `/shape/{app}/preview/{id}/` | `bashy shape preview <app> <id>` | web: preview URL; CLI: run the candidate binary; service: start it on a side port |
| `GET …/suggest` | `bashy shape suggest <app>` | |
| `POST`/`GET …/incidents` | `bashy shape incident <app> […]` | report a problem with the active version, which feeds a repair |
| (page actions) | `bashy shape history/undo/restore/release/offer/adopt` | thin calls into `bashy src` |
| **new** `POST …/submit` | `bashy shape submit <app> <rev> --story ID` | upstream through `src pr open` into the story's weave run, under the normal gates |

### App kinds

| Kind | First example | Activation scope | Verifier checks |
| --- | --- | --- | --- |
| web app (existing) | console, notepad, form | per user, served version | existing web verifier and protected parts (sign-in, pairing, credentials, terminal, API handlers) |
| CLI app (new) | bashy itself | per host: shaped binary in a side slot, release kept, PATH unchanged | build, then the app's declared test gate |
| service app (new) | Klatch | per host: unit points at the shaped image, data kept | build, tests, health check |

## Tracking (KISS/MVP)

- **Tracking is required and automatic.** `shape wish` (or `--from`) creates
  and links a todo, unless the caller links an existing story or todo with
  `--story`/`--todo`. Each change stays traceable with no extra step.
- **Shaped work is marked by origin, in the same workflow.** It has no separate
  process:
  - the todo or story gets the label `shaping`;
  - its frontmatter gets `origin: shape`, with the app, the candidate or version
    ID, the actor and the represented user;
  - submitted commits carry `Shape: <app>@<version>` and
    `Shaped-By: <actor>[ for <represented user>]` beside the existing
    `Sprint:`/`Story:`/`Story-ID:` trailers;
  - the story links to the candidate's verifier evidence.
- **Everything else is optional.** Filing it into a sprint, planning, review
  meetings and acceptance ceremonies are decided case by case by the human or
  the agent, as in a regular planning session. A todo needs no sprint.
- **Only existing gates apply.** `shape submit` merges through the normal review
  and merge gates; shaping adds no new mandatory gate.

## Rules

- **Isolation.** A candidate is always built in its own isolated workspace
  (`src preview` or weave), never in a shared checkout. Other agents' work is
  unaffected.
- **Scope.** Activation affects only its scope (one user for web apps, one host
  for CLI and service apps). Undo or restore returns to the previous or released
  version in one step.
- **No failing candidates.** A candidate that fails verification is never
  approved or activated.
- **Additive by default.** Removals and renames need the existing explicit
  contract step.
- **Parity.** Humans, agents and agent-assisted humans use the same verbs and
  authorization (Sprint 568 vocabulary: actor and represented user).
- **No change to web behavior.** Sprint 526's web shaping is unchanged.
  Sprint 564 adds the CLI front-end, the CLI and service kinds, and `submit`.
- **No satellite.** Shaping runs on the user's own host; Sprint 565 is not a
  dependency.

## Stories

| # | Story ID | Kind | Prio | Deliverable | Depends on |
| --- | --- | --- | --- | --- | --- |
| H1 | `a555c551303e` | requirement | P1 | One shaping model across web, CLI and service kinds; CLI ↔ API mapping; identity record | — |
| H2 | `80b272024354` | feature | P1 | `bashy shape` porcelain over `yoke/pkg/shape`, mirroring `/api/shape` | H1 |
| H3 | `67a325bc2095` | feature | P2 | CLI and service app kinds: registration, per-kind verifier, per-host activation and swap-back | H2 |
| H4 | `d25817fc29e4` | feature | P2 | `shape submit` (CLI and `POST …/submit`) into the sprint flow | H2 |
| H5 | `5f0412e066bd` | test | P2 | Verify on Linux, macOS and native Windows, plus a web regression check | H3, H4 |

## Relationship to other sprints

- **Sprint 570 (live handoff)** is the first bashy-self consumer: S570 H1 is
  H5's dogfood case. S570 reuses the offer/adopt/decline vocabulary and the
  notice mechanism. Not a hard dependency.
- **Sprint 563** keeps F14's provenance. The Klatch shaping is H3's service-kind
  worked example.
- **Sprint 568:** shaped apps keep the same accounts and identity semantics.

## Reviewed execution sequence (2026-11-05)

Planning update only. The eight stories are H1-H7 and spike 167b0f281c49;
H6 is done. H7 and the spike need goal-checklist coverage. H7 scenario B
also depends on H3. Keep the existing acceptance scope; milestones below
do not redefine full sprint completion.

1. **H1: settle the minimum contract.** Inventory existing web handlers,
   shape packages, src plumbing, registration and MCP capabilities. Specify
   the existing candidate lifecycle once, actor/represented-user resolution,
   local and remote authorization, CLI/API mapping and JSON results. Retain
   existing records; add only required identity/tracking fields. Auto-link a
   todo unless an existing item is supplied. No new planning ceremony.
2. **H2: deliver the web-app vertical slice first.** Extract only the
   candidate operations currently coupled to webconsole into a reusable
   shape service. Web handlers and local CLI call that service; remote CLI
   uses existing authenticated REST. Start status/wish/show/diff/verify/
   preview/approve/reject, then add remaining declared verbs and src wrappers.
   Preserve web behavior and gates. Exercise the Hello World example in a
   real browser through web and CLI. Audit its registration, contract and
   runner assumptions against actual runtime behavior before calling it
   supported. Hand-made --from tests isolate lifecycle checks from model
   variance; one real wish proves agent integration.
3. **H4: finish the useful web loop.** Add CLI and REST submit through src PR
   and weave, with matching origin/identity/evidence and checked trailers.
   Submit the retained approved Hello World revision after rollback. No new
   merge process, automatic merge or dependency on Sprint 570 handoff.
4. **H3: add app kinds one at a time.** Reuse existing selfbin/build/service
   machinery. First CLI: isolated build, declared checks, preview, explicit
   activation in a side slot, retained release, unchanged PATH, recoverable
   interrupted activation. Then Klatch service: isolated image build, tests,
   side-port health check, host-local image activation, data preserved and
   one-step rollback. Status identifies the active artifact. Never rebuild
   candidates in shared workspaces or activate failed/stale candidates.
5. **H5: validate one pinned candidate on three platforms.** Use existing
   self-hosted runners on Linux, macOS and native Windows. Exercise Klatch,
   bashy-self with Sprint 570 H1, failing candidates and original console
   web shaping. Hello World supplements but never replaces that console
   regression. If the H1 payload is unavailable, record that exact dependency;
   H7's help-text change does not substitute for H5 acceptance.
6. **H7: expand proven recipes, not the framework.** Start Codex/CLI Hello
   World, then Codex/CLI self-shaping (small help-text change, verify/preview/
   activate, inspect Hello World using the shaped command, restore, submit).
   Expand those two recipes across Codex/Claude/Muse/agy and CLI/MCP/REST:
   24 separately evidenced cells. Discover actual supported MCP operations;
   use existing authorized MCP command execution if sufficient, otherwise
   record the adapter gap as an explicit dependency. Agent narration and a
   direct HTTP-only harness do not establish agent E2E passes. Reuse H5 OS
   evidence rather than multiplying all 24 cells by three platforms.
7. **Spike 167b0f281c49:** inspect existing agentic-block behavior and write a
   concise concept/examples/gaps document. No syntax/runtime implementation;
   unresolved ideas become follow-up work, not a shaping delivery dependency.
   H6's accepted assets remain complete and need no redesign.

### Evidence and completion

Use focused shared-service/adapter tests for parity, identity, rejection,
wrong-scope denial, stale/failed verification and timeout/retry coherence.
Test interrupted CLI/service activation and recovery. Run relevant existing
web/src/selfbin regressions. Real-browser assertions cover baseline, preview,
active greeting, second principal and rollback; real agents actually invoke
selected interfaces. Preserve sanitized raw logs, source/tool/model/artifact
identities and verifier/browser evidence using existing runner conventions.
A generic agent task file or zero agent exit is not a test pass.

First usable milestone: H1 + H2 web lifecycle + H4 submission, with both
Hello World walkthroughs reproduced. Full sprint closure additionally requires
H3, H5, all H7 cells, spike documentation, and existing H6 evidence. Blocked
cells remain blocked. Close each story on its own criteria and relevant
regressions; do not claim release or publication authorization.

Deliver in dependency order with submodule commits/pushes followed by umbrella
pin bumps and required provenance trailers. Preserve unrelated concurrent
work. Link the spike and H7 into the goal checklist when the manager adopts
this plan; update H7's H3 dependency. This review does not take the sprint lease
or start feature implementation.
