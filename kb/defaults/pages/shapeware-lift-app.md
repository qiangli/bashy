---
id: 01a21d3c-8533-773f-84d1-020ea162fd3b
seq: 41
form: page
type: runbook
title: 'Lift an existing app or repo into shapeware: write its shapeware spec (reverse: source to spec)'
description: 'WHEN you lift an existing app, tool or repository into shapeware (convert it to a shapeware folder, make a repo shapeable, reverse SDLC, source to spec) so agents can rebuild, repair or extend it from the spec: choose the surface, SKILL.md and the dag.md lifecycle with effects and bands, features with acceptance lines, a contract with rules first and assertions from real runs, rules that define their sets, examples per format, known issues and tbd features, then compile with conform spec check, certify with cert and mutate, and prove it by a rebuild'
tags:
    - shapeware
    - lift
    - conform
    - spec
    - contract
    - reverse-sdlc
    - dag
status: candidate
evidence: '2026-11-26: examples/desk/form lifted this way (spec v2.1); its reference certifies 17/17 with 117/117 mutants; agents of four vendors rebuilt it from the spec, 3 of 3 runs each at the start tier. Renamed to lift 2026-11-27 (operator).'
source:
    tool: claude
created: "2026-11-27T13:40:35Z"
supersedes: shapeware-spec-from-app
valid_from: "2026-11-27T13:40:35Z"
---

**Lift** is the official word for this runbook's job: lifting an app turns its source into a shapeware spec so it can be shaped (formerly `shapeware-spec-from-app`).

Goal: lift an existing app or repository, making it shapeable: give it a shapeware spec that is sound enough for another agent to rebuild, repair or extend the app from the spec alone. Format: the shapeware spec (a superset of an Agent Skill folder; only `SKILL.md` is required, missing parts take defaults). Worked example, lifted this way and then rebuilt from its spec by agents of four vendors: bashy `examples/desk/form`. Related: [[shapeware-spec-from-plan]] when there are only instructions, [[shapeware-source-from-spec]] for the builder's side, [[shapeware-operate-from-dag]] to install and run a built folder, a rebuild test (a builder that has not seen the source rebuilds the app and the judge decides) to test the result.

Tools and format: the judge is `bashy conform` (`spec check`, `run`, `cert`, `mutate`). Where a step says `conform ...`, type `bashy conform ...`; on a machine without bashy use the standalone `conform` binary (github.com/dhnt/conform). The format on one page is `examples/desk/form/references/shapeware.md` in the bashy source, and the folder around it is the worked example, with a full `CONTRACT.md`, `features.yaml` and `dag.md`. `bashy src fetch bashy` puts the source under `${BASHY_HOME:-$HOME/.bashy}/src/bashy`.

## Before you start
- **Choose the surface, not the repository.** A spec covers what a user or agent can observe: commands, outputs, pages, files written. For a large repository, lift one app or one command family at a time.
- **Where the folder goes (fixed).** A self-contained app is its own folder (bashy `examples/desk/form`). A surface of a larger repository always goes in `shapeware/<surface>/` at the repository root, `<surface>` a short name for what is lifted; later surfaces are siblings. The first surface lifted in a repository also copies `shapeware/index.bsh` verbatim from bashy (`bashy src fetch bashy`; it lists, locates and lints every surface, and needs no editing), adds one line to the files agents already read (`CLAUDE.md`, `AGENTS.md`): "This repo has shapeware: run `bashy --bashsharp shapeware/index.bsh`", and adds `.shapeware/` to `.gitignore`.
- **You may read everything** (source, tests, README, help text): this is the reverse direction. What you may not do is copy implementation into the spec.
- **Find how it starts.** Does the first command work on a fresh HOME, or does a server have to be started first? Does it use a fixed port? Write that down; it decides the first assertions and the lifecycle.
- **Do builds, test suites, certification and mutation on a test host,** never on a shared development machine. Running the installed reference against loopback fakes in a throwaway HOME and TMPDIR, to capture what it prints, is allowed anywhere: that is running the reference, not building it.
- **When the lift is a sprint story,** assign it with its verify command and a runtime cap that covers certification, so the run can submit itself: `bashy sprint assign SPRINT STORY --verify 'bashy conform spec check shapeware/<surface> && bashy --bashsharp shapeware/index.bsh check' --max-runtime 3h`.

## Steps
1. **`SKILL.md` at the root:** the type line, the standard run block, and a short behaviour summary that points at the contract. For a surface of a larger repository (`shapeware/<surface>/`), use the lifted-surface variant of the run block (the format §4.3a): the "what it is" line naming the surface, the repository and the certified commit (`metadata.implementation`, `metadata.reference-commit`), and Build changes the implementation in a work copy of the repository, never this folder. Put how this particular build is operated (its scripts, server, workarounds) in a separate section marked as not required of other implementations.
2. **`dag.md`, the lifecycle:** the five steps `source`, `build`, `test`, `install`, `run`, plus `serve` and `stop` for an app with a page or a server. For a lifted surface, `source` is the two-way step of §4.3a: the containing repository when the folder sits in it, else a clone at the certified commit, recorded in `.shapeware/impl`; the other steps read `${IMPL:-$(cat .shapeware/impl)}`. Each step is exact and non-interactive, with an `Effects:` line and a `Band:` line (`none` where no model is needed, `unmeasured` until a drill measures it, `L1` to `L5` once measured). A `build` step calls the toolchain itself (`go build -tags ... -o ... ./cmd/...`), never a repository build script: such a script carries its own host preflights (outpost's `scripts/build.sh` demands `GO_CACHE_VOLUME_MAX_BYTES` under WSL and fails without it) and hides what the build needs. The lifecycle's effect check only warns about a command it cannot classify; it does not stop it. A certified folder also carries the standard gate targets `check`, `cert`, `mutate` and `alone` (the format §5b), so every later run is `bashy dag -f dag.md <target>`, not a hand-built command.
3. **`features.yaml`:** one feature per user-visible capability: `id`, `intent`, `effort` (Fibonacci points), `status` (`verified` only if you ran it; else `tbd`), `critical: true` for anything security-relevant, and `acceptance` lines of the form `command => expected`.
4. **`CONTRACT.md`, rules first:**
   - how assertions are judged (placeholders, open marker, what plain text is checked);
   - the output envelope and always-present keys;
   - every flag that works on several commands, defined in words;
   - open versus closed outputs, with the reason where closed protects content;
   - errors: the stream, the prefix, the exit code;
   - input definitions: every format, every allowed value.
   **A rule that names a set defines the set.** Never "names such as a, b, ...": give the decision rule, then a boundary pair, one case that must be refused and one near miss that must be accepted.
5. **`CONTRACT.md`, assertions from real runs.** Run each command on the reference and paste what it prints. Replace run-specific values with placeholders (`{ID}`, `{PORT}`, `{TIME}`, `{KEY}`). The judged output must be the command's own output: do not redirect it to a file and judge a `cat`, `jq` or `test` pipeline instead, or mutants of that command survive. Put a cross-step comparison in its own assertion that reads what an earlier step saved. A probe that gives the reference a throwaway HOME must also set `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_STATE_HOME` and `XDG_DATA_HOME` inside it: `--fresh-home` sets them, and a program that prefers them never reads the HOME the probe prepared. One assertion per behaviour, in dependency order. Give the first create step a fixed id where the app allows it, so one mismatch does not unbind every later step.
6. **UI as checkable rules:** one control per field, a programmatic label, how required fields are marked, allowed control kinds, button names; a `ui` assertion on the page inventory; one page interaction (fill by label, submit, read back through the CLI).
7. **`examples/`:** one worked file per input format, each used by an assertion, covering the syntax actually accepted.
8. **Known issues and unbuilt features, honestly:** a real bug is a known-issue entry (status, the feature, symptom -> action, retry bound, expiry); something planned and not built is a `tbd` feature with acceptance lines.
9. **Compile the spec:** `conform spec check DIR` until it reports nothing. It checks features without acceptance lines, example files nothing uses, flags never defined, outputs with no open/closed rule, rules given by example, and the lifecycle steps with their effects and bands.
10. **Certify the reference on the test host:** `conform cert` (3 runs, every assertion passes, none noisy) and `conform mutate` (every mutant killed). A surviving mutant is a missing assertion: add it and repeat. A contract that asserts only refusals (every command against a missing app) certifies and kills every mutant while proving nothing the features claim: a reviewer reads the assertions, not only the cert and mutate summary, and a feature is `verified` only when an assertion exercises its success path. Also run the app's own acceptance checks. Name the shimmed command with `--bin`: conform otherwise takes the first word of each assertion, so a contract that drives the app through a script (`python3 probe.py ...`) certifies the wrong program. Size mutation before starting it (mutants × one contract run ÷ shards) and record the estimate. Run long jobs in the foreground of a session that outlives them: a job left running on a remote host dies with its login session, and a worker's background jobs die when its run reaches the runtime cap.
11. **State the venue** a rebuilt app must run on: OS, interpreter and toolchain versions, network, what is absent.
12. **Prove it reproduces.** Have a builder that has not seen the source rebuild the app from the spec (the rebuild test). Classify every failure. A failure caused by the spec goes back to step 4; expect one or two rounds.
13. **For a lifted surface, prove the folder stands alone.** Copy only `shapeware/<surface>/` somewhere outside any checkout, give it to an agent that has not seen the repository, and require a passing **Check** from the folder's own text and lifecycle.
14. **Record what was measured:** the lowest band that rebuilt it, per lifecycle step where measured, with the bindings and the date.

## What the first conversion taught
- The first spec read well and reproduced badly: 1 pass in 15 rebuild attempts. After the gaps were closed, start tiers of four vendors passed. The gaps were all of a few kinds: a format named in prose but never used by an assertion; a flag used in examples but never defined; outputs closed for no stated reason; page rules left to inference; the behaviour spec mixed with how the reference is operated; a rule given by example.
- **Run every case you write down.** Reading the code suggested an unknown field kind gives one error; running it showed the kind `secret` gives another. Only the run is evidence.
- **The app may differ from its own documentation.** Record the app's real behaviour in the contract and file the difference as a known issue; do not write the spec to match the documentation.
- **Do not ask a builder to re-create a critical rule from examples.** Supply it as data, or define it exactly.
- **Hidden cases stay with the owner.** They vary documented behaviour only, and each is independent (its own state, a fresh HOME).

## What the second conversion taught (a service app; the spec written by an agent, the hidden cases by another vendor's agent)
- **A service needs a start and a stop that the contract can run.** If the app has no stop verb, or no verb starts its server, say where they live (a small launcher beside the built program is acceptable), how the contract starts the server and waits until it is ready, and what `stop` is scoped to (this folder or this user). Assert that `stop` exits 0 with nothing running and that stored state survives a stop. A signal is not a stop the contract can check.
- **Name the reference command.** Certify the command a user runs (the launcher), not the program it wraps: `conform cert --reference-cmd` takes that command, and `--dir` the folder that holds `examples/`.
- **Know how the judge compares output.** Only a command that carries `--json` is compared structurally (open markers, any key order). Every other output is compared as text, including JSON printed by `curl`. For those, either state the exact format as a rule or normalise the output inside the command. A byte format the spec never stated failed a build whose behaviour was right.
- **When the reference's page fails the UI rules** (no programmatic labels, for example): assert what the page does carry (the visible texts, the button names) with a console check, record the gap as a known issue, and say that an implementation may do better.
- **A surviving mutant is usually a missing assertion, not always.** Redirections, pipes and compound commands can hide a mutation from the judge. First make the assertion's last step one plain command; add an assertion only if the mutant still survives.
- **A rule with no assertion is unverified.** A second agent found four places where the reference did not follow rules its contract stated, and no assertion covered any of them. Run every `features.yaml` acceptance line on the reference; for each rule in the contract, point at the assertion that exercises it, or move it to the open questions.
- **Binary examples.** When no fixture exists, make one, say in the spec how it was made, and assert properties that do not depend on one extractor's exact text. A tool the app calls for a conversion is a venue line, not an implementation detail.
- **Both lifecycle files pass the lint.** In a reference folder `conform spec check` reads the `dag.md` beside `CONTRACT.md`. The lifecycle of the app as a spec (kept as `spec/dag.md`, with `spec/README.md` listing the files of a spec release) is checked when the release is assembled. Steps of the spec's lifecycle run with `sh` and need nothing of the reference build.
- **Say which state model the contract uses.** Ordered assertions against one server and one data folder suit a service: state it under "How assertions are judged", start the server in the first assertion and stop it in the last. Independent assertions, each in a fresh HOME, suit a tool with no server.

## Sizing
One small app (about 2,000 to 3,000 lines, one service, one page): one working session for the spec and its certification, plus a rebuild round. Certification with mutants took about 35 minutes of test-host time per pass in the worked example. A product surface (Sprint 737): 20 to 80 minutes of agent time; the largest, 559 mutants at about 75 seconds per contract run, took about 90 minutes of test-host time over 8 shards.
