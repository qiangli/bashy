---
id: 01a21822-3b2a-7776-a940-ff8be548de53
seq: 37
form: page
type: runbook
title: 'Make an existing app or repo shapeable: write its shapeware spec (reverse: source to spec)'
description: 'WHEN you convert an existing app, tool or repository into a shapeware folder so agents can rebuild, repair or extend it from the spec (reverse SDLC, source to spec, make a repo shapeable): choose the surface, SKILL.md and the dag.md lifecycle with effects and bands, features with acceptance lines, a contract with rules first and assertions from real runs, rules that define their sets, examples per format, known issues and tbd features, then compile with conform spec check, certify with cert and mutate, and prove it by a rebuild'
tags:
    - shapeware
    - conform
    - spec
    - contract
    - reverse-sdlc
    - dag
status: candidate
evidence: '2026-11-26: examples/desk/form converted this way (spec v2.1); its reference certifies 17/17 with 117/117 mutants; agents of four vendors rebuilt it from the spec, 3 of 3 runs each at the start tier'
source:
    tool: claude
    host: dragon
created: "2026-11-26T13:53:46Z"
---

Goal: make an existing app or repository shapeable: give it a shapeware spec that is sound enough for another agent to rebuild, repair or extend the app from the spec alone. Format: the shapeware spec (a superset of an Agent Skill folder; only `SKILL.md` is required, missing parts take defaults). Worked example, converted this way and then rebuilt from its spec by agents of four vendors: bashy `examples/desk/form`. Related: [[shapeware-spec-from-plan]] when there are only instructions, [[shapeware-source-from-spec]] for the builder's side, a spec-to-source drill (a builder that has not seen the source rebuilds the app and the judge decides) to test the result.

## Before you start
- **Choose the surface, not the repository.** A spec covers what a user or agent can observe: commands, outputs, pages, files written. For a large repository, convert one app or one command family at a time; a root `SKILL.md` can make the whole repository a shapeware folder later.
- **You may read everything** (source, tests, README, help text): this is the reverse direction. What you may not do is copy implementation into the spec.
- **Find how it starts.** Does the first command work on a fresh HOME, or does a server have to be started first? Does it use a fixed port? Write that down; it decides the first assertions and the lifecycle.
- **Do builds, tests and runs on a test host,** never on a shared development machine.

## Steps
1. **`SKILL.md` at the root:** the type line, the standard run block, and a short behaviour summary that points at the contract. Put how this particular build is operated (its scripts, server, workarounds) in a separate section marked as not required of other implementations.
2. **`dag.md`, the lifecycle:** the five steps `source`, `build`, `test`, `install`, `run`, plus `serve` for an app with a page. Each step is exact and non-interactive, with an `Effects:` line and a `Band:` line (`none` where no model is needed, `unmeasured` until a drill measures it, `L1` to `L5` once measured).
3. **`features.yaml`:** one feature per user-visible capability: `id`, `intent`, `effort` (Fibonacci points), `status` (`verified` only if you ran it; else `tbd`), `critical: true` for anything security-relevant, and `acceptance` lines of the form `command => expected`.
4. **`CONTRACT.md`, rules first:**
   - how assertions are judged (placeholders, open marker, what plain text is checked);
   - the output envelope and always-present keys;
   - every flag that works on several commands, defined in words;
   - open versus closed outputs, with the reason where closed protects content;
   - errors: the stream, the prefix, the exit code;
   - input definitions: every format, every allowed value.
   **A rule that names a set defines the set.** Never "names such as a, b, ...": give the decision rule, then a boundary pair, one case that must be refused and one near miss that must be accepted.
5. **`CONTRACT.md`, assertions from real runs.** Run each command on the reference and paste what it prints. Replace run-specific values with placeholders (`{ID}`, `{PORT}`, `{TIME}`, `{KEY}`). One assertion per behaviour, in dependency order. Give the first create step a fixed id where the app allows it, so one mismatch does not unbind every later step.
6. **UI as checkable rules:** one control per field, a programmatic label, how required fields are marked, allowed control kinds, button names; a `ui` assertion on the page inventory; one page interaction (fill by label, submit, read back through the CLI).
7. **`examples/`:** one worked file per input format, each used by an assertion, covering the syntax actually accepted.
8. **Known issues and unbuilt features, honestly:** a real bug is a known-issue entry (status, the feature, symptom -> action, retry bound, expiry); something planned and not built is a `tbd` feature with acceptance lines.
9. **Compile the spec:** `conform spec check DIR` until it reports nothing. It checks features without acceptance lines, example files nothing uses, flags never defined, outputs with no open/closed rule, rules given by example, and the lifecycle steps with their effects and bands.
10. **Certify the reference on the test host:** `conform cert` (3 runs, every assertion passes, none noisy) and `conform mutate` (every mutant killed). A surviving mutant is a missing assertion: add it and repeat. Also run the app's own acceptance checks.
11. **State the venue** a rebuilt app must run on: OS, interpreter and toolchain versions, network, what is absent.
12. **Prove it reproduces.** Have a builder that has not seen the source rebuild the app from the spec (a spec-to-source drill (a builder that has not seen the source rebuilds the app and the judge decides)). Classify every failure. A failure caused by the spec goes back to step 4; expect one or two rounds.
13. **Record what was measured:** the lowest band that rebuilt it, per lifecycle step where measured, with the bindings and the date.

## What the first conversion taught
- The first spec read well and reproduced badly: 1 pass in 15 rebuild attempts. After the gaps were closed, start tiers of four vendors passed. The gaps were all of a few kinds: a format named in prose but never used by an assertion; a flag used in examples but never defined; outputs closed for no stated reason; page rules left to inference; the behaviour spec mixed with how the reference is operated; a rule given by example.
- **Run every case you write down.** Reading the code suggested an unknown field kind gives one error; running it showed the kind `secret` gives another. Only the run is evidence.
- **The app may differ from its own documentation.** Record the app's real behaviour in the contract and file the difference as a known issue; do not write the spec to match the documentation.
- **Do not ask a builder to re-create a critical rule from examples.** Supply it as data, or define it exactly.
- **Hidden cases stay with the owner.** They vary documented behaviour only, and each is independent (its own state, a fresh HOME).

## Sizing
One small app (about 2,000 to 3,000 lines, one service, one page): one working session for the spec and its certification, plus a rebuild round. Certification with mutants took about 35 minutes of test-host time per pass in the worked example.
