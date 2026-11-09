---
id: 01a1c1f5-84d7-798d-b54c-bfe4ff42bf28
seq: 18
form: page
type: decision
title: 'Extend bashy: small verbs, Bash# skill-scripts, shaping hooks, and the example → optional → core levels'
description: 'How every agent adds or changes bashy features, runbooks and shaped apps: compose small JSON verbs in Bash# skill-scripts, design a shaping hook, start new helpers as examples and graduate them to optional then core, keep shared services provider-agnostic — WHEN adding a command, writing a runbook/kb page, planning a feature, or shaping an app (keywords: rod not fish, unix, compose, skill-script, bsh, shaping, extension point, example, optional, core, alpha, beta, graduate, maturity, experimental, preview, supported, provider-agnostic)'
tags:
    - extending
    - shaping
    - bashsharp
    - levels
    - guidelines
status: candidate
evidence: Operator guideline 2026-11-09 (lern-psic sprint planning); tiers from docs/release-roadmap-and-versioning.md
source:
    tool: claude
created: "2026-11-09T20:17:35Z"
---

## Rules

Every agent extending bashy follows these, whether it is coding in a bashy repo, writing a runbook, or shaping an app at runtime with `bashy shape` / the shaping button.

1. **Grow bashy, don't work around it.** No command for the job: register one (`bashy commands add`). One exists but doesn't fit: shape it (`bashy shape wish …`).
2. **Small verbs, composed (rod, not fish).** Build verbs that do one thing, read and write plain files or JSON (`--json`), and compose in pipelines and scripts. Higher-level helpers are thin compositions of those verbs, never a monolith; each step stays usable alone. Kb pages teach the verbs and the human steps, not one canned flow.
3. **Compose in Bash# skill-scripts.** A composition is one `.bsh` file that is both the command and its skill (a `~~~skill as guide` fence), registered with `bashy commands add` (or the bashy-defaults list). Write Go only for a verb a script cannot do.
4. **Design for shaping.** Every user-facing surface is shapeable or has a declared extension point: a template, a JSON/YAML config, or a skill-script verb that shaping can change later. A shaped change is the smallest one that satisfies the request and adds with a fallback rather than renaming or removing (expand, then contract).
5. **Three levels, one graduation path.** Where a feature lives follows its stability tier (`experimental → preview → supported`, see the release roadmap):

| Level | Tier | Bar |
|---|---|---|
| example (`examples/`) | experimental (alpha) | Quick implementation; may miss an OS or feature; any common tech allowed (cgo, Node, …); never linked into the core binary |
| optional add-on | preview (beta) | All three OSes (Windows, macOS, Linux); installed separately or opt-in; not in the core binary |
| core | supported (release) | Foundation rules: pure Go, permissive licenses only, `CGO_ENABLED=0`, cross-OS tests |

New user-facing helpers start as examples. A feature graduates in a release on evidence (it works on the next level's platforms and has a named consumer), never on age; demote or remove instead of carrying an example forever. Things a shaping agent adds to an app are example level until promoted.

6. **Provider-agnostic.** Shared services (LLM door, metering, storage, …) work with any backend, including local ones such as ollama. A vendor backend (e.g. a cloud LLM) is a configuration, not a code path the feature depends on.

## Check before you build

- Search first: `bashy commands --agentic`, `bashy kb search "<task>"`, `bashy skill list`.
- Name the level you are building at and the verbs you compose.
- Name the shaping hook (which file or verb a user can shape).
