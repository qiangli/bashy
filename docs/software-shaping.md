# Software shaping

**Software shaping lets anyone tell an agent, in plain words, how a running
tool should work, and get their own verified, reversible version, while
everyone else keeps theirs.**

This page is the canonical definition. Every README in the family (bashy,
[Bash#](https://github.com/bashsharp/bashsharp), [Yoke](https://github.com/qiangli/yoke),
[outpost](https://github.com/qiangli/outpost), [bash](https://github.com/qiangli/bash))
quotes the sentence above and links here. The implementation contract is
[shape-contract.md](shape-contract.md); the user walkthrough is
[overview.md § Software Shaping](overview.md#software-shaping-make-your-tools-fit-you).

## The loop

```text
wish → candidate → verify → preview → approve (for me) → undo / restore
                                                  └──→ promote: local → team → upstream
```

1. **Wish.** Describe the change in plain words, or import a hand-made workspace (`--from DIR`).
2. **Candidate.** An agent writes the change in an isolated workspace. Nothing running changes.
3. **Verify.** Ordinary commands and declared checks decide whether the candidate is good. A model's opinion does not count.
4. **Preview.** Try it: a web URL, a CLI argv, or a service health check, all isolated.
5. **Approve.** Activate it for yourself. Approval re-runs verification first.
6. **Undo / restore.** Go back one step, or to any earlier revision.
7. **Promote.** Offer an approved change to your team, then upstream. It arrives on their side as a new wish, so it goes through the same gates.

The same verbs drive it from the CLI and the web console:

```bash
bashy shape wish APP "make the list sortable by date" --json   # → candidate id
bashy shape verify APP ID --json                               # trusted verifier
bashy shape preview APP ID --json
bashy shape approve APP ID --json                              # active for me only
bashy shape undo APP --json                                    # the way back
bashy shape push --to org                                      # promote to the team
```

## Guidelines

A thing is shaped *correctly* only when all eight hold.

1. **Yours, not everyone's.** Activation is scoped: per principal for web apps, per host for CLI tools and services. Other users and hosts keep their version.
2. **Judged, never trusted.** An agent's output is only a candidate. Commands and checks decide. Approval re-runs verification, and a cached verdict is never authority. A failed check can't be approved.
3. **Always a way back.** Every activation is a revision. Undo is one step, and restore can return to any revision.
4. **The core does not move.** Shaping changes registered apps, tools, configs and skill-scripts. It never changes the shell, the userland or the signed core that verifies it. The thing doing the judging is not up for shaping.
5. **Contracts, not prose.** A shape may add surface (routes, flags, args, JSON keys) freely. It may remove surface only through an explicit contract step, and it may never silently escalate effects such as network, write or exec.
6. **Rod, not fish.** Each step is a small `bashy shape … --json` verb with meaningful exit codes (0 nothing pending, 3 pending, 1 error). Higher-level flows are short Bash# skill-scripts that compose those verbs, and the scripts can themselves be shaped.
7. **UI → CLI parity.** Every web action has a CLI verb on the same entities, with the same permissions and the same JSON. A CLI verb does not need a web page.
8. **Provenance.** Every wish is tracked as a todo (`origin: shape`). Commits carry `Shape: APP@VERSION` and `Shaped-By:`, and verification evidence travels with the candidate.

## Technologies: what makes it safe

| Layer | Role in shaping | Where |
|---|---|---|
| **Classic**: pure-Go GNU Bash 5.3 + POSIX.1-2016 userland | The fixed base. The same script means the same thing on Linux, macOS and Windows, so a check that passes here passes there. | [qiangli/bash](https://github.com/qiangli/bash) = [sh](https://github.com/qiangli/sh) + [coreutils](https://github.com/qiangli/coreutils) |
| **Bash#** | The language shapes and checks are written in. One file is both script and skill; `@require` / `@ensure` / `@guard` contracts and `agentic {}` blocks judge a model's output with ordinary commands. | [bashsharp](https://github.com/bashsharp/bashsharp) |
| **Yoke** | The shaping engine: the candidate lifecycle, verifier, evidence, scoped activation and history (`pkg/shape`), the web console, and isolated workspaces (`weave`). | [yoke](https://github.com/qiangli/yoke) |
| **bashy** | The one binary: the `bashy shape` CLI, the web console at `/shape/<app>/`, `dag` gates, and the agent surface (`--json`, MCP). | this repo |
| **outpost** | Brings your own machines and the apps on them behind one door, so they can be shaped and shared. | [outpost](https://github.com/qiangli/outpost) |
| **Satellite** | A server you or your team run, where shaped versions are shared with a circle or team and promoted to everyone in it. | hosted; reached through the public protocol only |

## Features: what ships, what is in progress

| Feature | Status | Evidence |
|---|---|---|
| CLI lifecycle: wish, verify, preview, approve, reject, undo, restore, submit | Shipped since v1.0.0-rc.27 | [release notes](releases/v1.0.0-rc.27.md) |
| Promotion: remote, outgoing, push, incoming, pull | Shipped since v1.0.0-rc.27 | [overview](overview.md#the-promotion-path) |
| Web console shaping, scoped to a principal | Opt-in trial | [shaping-trial.md](shaping-trial.md) |
| Hello World walkthrough (web and CLI) | Shipped example | [examples/shaping/hello-world](../examples/shaping/hello-world/README.md) |
| Cross-platform shaping validation | In progress | [known issues](releases/v1.0.0-rc.27-known-issues.md) |
| Test-first shaping (red on base, green on candidate, refactor without drift) | Planned | Sprint 621 |
| Shaping bashy itself from a reproducible source install | Planned | Sprints 617–618 |
| Shaping from a free cloud workspace | Planned | Sprint 615 |

Only rows marked shipped are product claims. Each one names its evidence.

## "Shapeable with Bashy"

The badge and the **Shape** action use the Flow mark from the
[design archive](software-shaping-design/README.md). An app earns the badge only
when every item below holds. The checklist decides, not the app's own description:

- [ ] It is registered (`bashy app add --type shaped`) with a declared kind: `web`, `cli` or `service`.
- [ ] It declares checks the verifier runs, and at least one of them can fail.
- [ ] Activation is scoped to a principal or a host, never global.
- [ ] Undo and restore work, and history lists each revision.
- [ ] Every web action has its `bashy shape` CLI equivalent.

Later sprints extend this checklist. They add rows here rather than defining shaping again elsewhere.

## What shaping is not

- **Not a code assistant.** The result is a running version you can approve, not a diff you have to merge.
- **Not self-modification.** An `agentic {}` block grants no authority to change an app ([boundary](agentic-block-shaping.md)).
- **Not a fork.** Your version stays on the same history as everyone else's, so it can be promoted, pulled or undone.
