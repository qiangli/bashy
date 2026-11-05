# Sprint 564 master plan: software shaping for Bashy apps

Sprint UUID: `0f09b646-9acd-5bec-849d-2cf3b0e4edef`. Planning only: this doc
rectifies the card and stories, and authorizes no implementation. The card and
its stories are authoritative.

Origin: Sprint 563 finding F14 (`plan-s563-master-execution.md`) recorded a
manual shaping of Klatch on the Windows host: host git, two podman builds, an
image swap and a secret copy. The shaping commits are `469031ce` and `0fe385ad`
on the Klatch shaping branch. The S562 handoff retro (`retro-s562-manager-handoff.md`)
showed the same need for bashy itself: a feature such as live handoff
(Sprint 570) could be shaped and used in real time instead of only being filed.

## Goal

Any Bashy user, a human or an agent, or a human assisted by an agent, can shape
an app on their own host:
1. start from the pinned commit (or main) in an isolated workspace,
2. change it, then build and test it there,
3. swap the shaped build in on that host only,
4. swap back to the release in one step.

The shaped source and build identity are recorded throughout, and the user can
hand the result into the normal sprint flow for review and merge.

## Rules

- **Isolation.** Shaping always happens in a weave workspace, its own clone on
  branch `shape/<id>`, never in a shared checkout. Other agents' work is
  unaffected.
- **Scope of a swap.** A swap affects only the current host and user. The
  pinned release stays installed as the fallback.
- **No failing builds.** A shaped build that fails its declared tests is never
  used.
- **No satellite.** Shaping runs on the user's own host; Sprint 565
  (single-node outpost, J1/J8) is not a dependency.
- **KISS.** Compose the existing commands: `weave` (workspace, branch),
  `self build`/`self install`, `service`, `tool install`/pin. No new framework
  or store. `--json` everywhere. Humans and agents use the same commands and
  authorization.

## App kinds

| Kind | First example | Build | Swap |
| --- | --- | --- | --- |
| CLI tool | bashy itself | `self build` in the workspace | side slot for the shaped binary, release preserved, PATH unchanged |
| Service app | Klatch (F14) | the app's build (container image) | `service` unit points at the shaped image; data kept |

## Proposed surface (for H1 to confirm)

```
bashy shape start <app> [--story ID]   # weave workspace at pin (or main), branch shape/<id>
bashy shape test                        # the app's declared gate, in the workspace
bashy shape use                         # swap in the shaped build on this host; release kept
bashy shape release                     # one-step swap back to the pinned release
bashy shape status                      # active slot, shaped source/build identity, test verdict
bashy shape submit [--story ID]         # branch -> weave run linked to the story -> review/merge
```

## Stories

| # | Story ID | Kind | Prio | Deliverable | Depends on |
| --- | --- | --- | --- | --- | --- |
| H1 | `a555c551303e` | requirement | P1 | Define the shaping workflow for CLI and service apps | — |
| H2 | `80b272024354` | feature | P1 | `bashy shape` verb composing existing commands (start/test/status/release) | H1 |
| H3 | `67a325bc2095` | feature | P2 | Swap and swap-back for both kinds | H2 |
| H4 | new | feature | P2 | `shape submit` into the sprint flow | H2 |
| H5 | new | test | P2 | Verify on Linux, macOS and native Windows | H3, H4 |

H3 was previously the verification story. It now carries the swap mechanics,
and verification moves to H5.

## Relationship to other sprints

- **Sprint 570 (live handoff)** is the first consumer. Its H1 (`handoff show`)
  is the bashy-self dogfood case for H5. S570 can still be delivered through the
  normal weave flow, so this is not a hard dependency.
- **Sprint 563** keeps F14's provenance. Its manual Klatch shaping is H1's
  service-app worked example.
- **Sprint 568**: a shaped app keeps the same accounts and identity semantics;
  no identity work here.
