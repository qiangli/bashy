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
