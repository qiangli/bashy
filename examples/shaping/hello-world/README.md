# Hello World: Software Shaping

One tiny app, two walkthroughs, and the canonical Sprint 564 H7 E2E A fixture.
The page has no external dependencies or user data. Its initial heading is
exactly **Hello, World!**. The smoke check preserves a greeting heading without
freezing its words; lifecycle tests assert the requested words independently.

## Setup (both walkthroughs)

Use a Bashy build supporting shaped web apps and a configured shaping agent.
Run `bash register.sh /path/to/new/hello-world-source` from this directory,
choosing a new directory outside this repository. The script copies only app
files and creates an independent Git checkout. Keep it for the app’s lifetime. Registration deliberately omits
`--force`; inspect an existing registration instead of overwriting it.
Start `bashy app serve --bind lan --pair --shape-agent <tool:model>`, replacing the agent
placeholder with your configured agent. Pair a browser using
`bashy app pair --principal <you> --allow hello-world` and follow the printed
URL. Open Hello World from the launcher. Confirm the original heading.
The app itself needs no model; generating a shaping candidate does.

## Web walkthrough

1. Open `/shape/hello-world/` on that console.
2. Request: **Change the displayed greeting from “Hello, World!” to “Hello, Shaping!”.**
3. Inspect the candidate and diff, run Verify, and open Preview.
4. Confirm the preview says **Hello, Shaping!** while the active app still says
   **Hello, World!**. Approve explicitly.
5. Reopen the app and confirm **Hello, Shaping!** and the active version in the
   shaping page. Another paired principal must still see **Hello, World!**.
6. Use Undo or restore the base version. Confirm **Hello, World!** returns.

## CLI walkthrough

This walkthrough requires Sprint 564 H2's CLI. It is an acceptance recipe,
not a claim that the CLI exists in every released build. Check
`bashy shape --help` before starting; follow its authentication/scope options.

```sh
bashy shape status hello-world
bashy shape wish hello-world 'Change the displayed greeting from "Hello, World!" to "Hello, Shaping!".'
# Replace CANDIDATE with the returned candidate ID.
bashy shape show hello-world CANDIDATE
bashy shape diff hello-world CANDIDATE
bashy shape verify hello-world CANDIDATE
bashy shape preview hello-world CANDIDATE
bashy shape approve hello-world CANDIDATE
bashy shape status hello-world
bashy shape history hello-world
bashy shape undo hello-world
```

Use the browser for the same before/preview/after/second-principal/undo checks
as the web walkthrough. Never approve a failed verifier result. Record IDs
from actual output rather than assuming names or version numbers.

## Submit the change (optional, H4)

Keep the approved version ID before undoing. With H4 available, use
`bashy shape submit hello-world VERSION --story STORY_ID` against your
own disposable test story/run. Inspect the resulting proposal and normal
gates. Submission does not mean approval or automatic merge.

## Repeatable acceptance and recording

H2 checks interface parity; H4 checks submission; H5 reuses the web regression;
H7 runs its existing agent/interface matrix using this app. Each run copies
this pinned directory to a disposable workspace and registers it in isolated
Bashy state. Never change this shared example to execute a test.

Assert visible greetings in a real browser, candidate and active identities,
principal isolation, rejection leaving the active version unchanged, and
failed verification blocking approval. For a broken candidate, remove
`public/index.html` in an isolated candidate workspace; do not edit deployment.
Keep tracking, source revision, tool/interface versions, sanitized logs and
verifier evidence. Unsupported integrations are blocked, not passing.
A deterministic fixture check is `bashy -c 'cd "$1" && sh checks/smoke.bsh' -- .`.
It needs no model and does not replace browser or agent E2E evidence.

For a short demo recording, show the original, request, preview, explicit
approval, changed app, and undo. Use synthetic principals. Record the tested
Bashy and example revisions. Public posting is a separate step.

## Agent E2E

[e2e-agent.md](e2e-agent.md) is the executable task specification for H7's CLI
cell. Prepare an isolated registration, two paired principals, browser access
and a disposable submission story. Configure those details in the agent's test
session, then run `bash e2e-agent.sh codex exec -` (or another actual agent
command that reads stdin). Set `BASHY_BIN` to the candidate executable and make
that same executable available as `bashy` in the agent's PATH.
The runner exits 2 when the shaping CLI is unavailable. Capture sanitized
agent logs and browser artifacts using the existing test runner. A zero agent
exit code does not establish an E2E pass; review the task's evidence checklist.
MCP/REST cells use H7's supported adapters once available, not invented tools.
