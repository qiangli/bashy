# Preparing real-agent H7 cells

This extends the existing `e2e-agent.sh` stdin launcher. It prepares no models,
credentials, app state or passing results. Each of four tools, three interfaces
and two scenarios needs a separately reviewed result: 24 cells. The launcher
exits with the agent's status; zero means the process finished, not E2E PASS.

## One disposable installation per cell

Pin the candidate Bashy executable, matching dependencies and fixture revision.
Create a fresh workspace outside shared checkouts, isolated `BASHY_HOME`, and
isolated catalog/config roots using the candidate's documented environment.
Set `BASHY_FLEET_DIR` to the disposable catalog; clear inherited noun-specific
`BASHY_*_DIR` overrides or explicitly bind them inside that catalog. Otherwise
an app registration can still write shared configuration despite BASHY_HOME.
Set a private PATH directory containing that same candidate as `bashy`; keep
the released executable and PATH on the host unchanged. Do not reuse an active
self-shaping slot across cells. Register a fresh Hello World source with
`register.sh`, create a disposable submission story in the workspace, and
prepare two distinct paired synthetic principals in fresh browser profiles.
Use the existing app serve/pair commands and normal agent bindings. Selecting
an agent tool must not install or silently change its model/authentication.

For scenario B, additionally use a clean pinned product build and matching Git
source (preserve its commit; folder reimport creates a different identity).
Register the trusted CLI build/test gate and retained release using H3's public
registration contract. `BASHY_SRC_COMMIT` is development metadata and cannot
prove dirty executable bytes match a release. Keep the baseline Hello World
app available for the shaped tool's read-only inspection.

Write a private `E2E_CONTEXT` file containing resource references: candidate
and fixture commit/digest, app name, principal names, console origin, paired
browser profiles, disposable story ID/store, evidence directory, chosen model
and selected MCP server name or REST session file path. Include no cookie,
token or credential values. Agents retain their normal provider auth outside
the disposable app state; do not copy provider auth into public artifacts.
Run from the disposable workspace, so auto tracking cannot touch this repo.

```sh
export BASHY_BIN=/path/to/pinned/bashy
export BASHY_HOME=/path/to/disposable/state
export BASHY_FLEET_DIR=/path/to/disposable/catalog
export E2E_CONTEXT=/path/to/private/run-context.md
export E2E_SCENARIO=hello-world # or self-shape
export E2E_INTERFACE=cli       # or mcp, rest
bash /path/to/example/e2e-agent.sh codex exec --json -
```

The same stdin contract supports `claude -p --output-format stream-json
--verbose`. Check installed help before choosing arguments for other tools:
Muse supports `exec --prompt-file` (use `/dev/stdin` on Unix or an operator
wrapper accepting stdin), while agy supports print text stdin. Use Bashy's
registered real-agent launcher when an agent's native CLI does not accept
stdin. Do not replace a missing invocation with an echo provider. Capture the
actual executable/model version; installed and registry versions can differ.

## MCP and remote hosts

Configure the real agent's run-scoped MCP server with the candidate command
`bashy mcp serve --tools shape --allow destroy,spend`; discover its actual
tools/list and confirm the `shape` tool before starting. Scope policy to the
disposable installation. Avoid editing shared agent configuration merely to
run a cell. A stdio MCP command may use SSH to launch the pinned candidate on
an isolated remote host, with explicit remote working directory, state/config
and environment. The agent must issue actual tools/call events. SSH CLI calls
alone are CLI evidence, not MCP evidence. Use the same remote console through
a loopback tunnel for browser observation and REST, preserving paired scope.
Do not put cookie values in SSH arguments, process logs or prompts.

## Browser evidence and review

Reuse the webconsole `verifydom` chromedp harness and its browser allocator
policy. Chrome may be installed as an application bundle even when absent
from PATH. Its current `domEnv` starts an in-process test server: those tests
prove web behavior but cannot by themselves prove what a separately launched
agent changed. Agent acceptance needs the existing harness attached to the
actual disposable console, or an agent-accessible real browser integration.
Record heading visibility and active identity before, in preview, after
approval, under the other principal, and after rollback, plus screenshots and
browser errors. An HTTP body check or model narration cannot replace this.
Keep the browser's sandbox policy consistent with the existing harness.

Record each cell as NOT RUN until execution, then PASS, FAIL or BLOCKED with
reason and raw evidence links. Include exact selected-interface events,
candidate/active/source/artifact identities, verifier results, actor and
represented user, tracking proposal/run, browser assertions, and reused H5
platform evidence keyed to the identical candidate. Negative verifier,
authentication/scope, timeout/interruption and retry cases remain separate
required H7 evidence; the happy-path task alone does not close the story.
Review/sanitize private raw logs before publishing. Clean up only the cell's
own processes, browser profiles and disposable resources.
