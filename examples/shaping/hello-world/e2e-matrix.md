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
For isolated execution, give Bashy itself a disposable operator `HOME` as
well: weave queues and some coordination/telemetry stores use the host home
rather than `BASHY_HOME`. A run-scoped executable wrapper can set that HOME,
`BASHY_COORD_DIR` and `BASHY_OTEL_SPOOL` before executing the pinned real binary.
Keep the actual agent CLI on its normal provider HOME. If a console's configured
shaping agent needs that provider login, reference its existing private profile
from the operator home; never copy credentials into app state, prompts or evidence.
Record both the wrapper and real executable path/digest. Do not grant a test
write access to shared queues merely to get past a sandbox refusal.

Set a private PATH directory containing that same candidate as `bashy`; keep
the released executable and PATH on the host unchanged. Do not reuse an active
self-shaping slot across cells. Register a fresh Hello World source with
`register.sh`, prepare a writable disposable tracking store in the workspace, and
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
browser profiles, disposable tracking store (and optional story ID), evidence directory, chosen model
and selected MCP server name or REST session file path. Include no cookie,
token or credential values. Agents retain their normal provider auth outside
the disposable app state; do not copy provider auth into public artifacts.
Run from the disposable workspace, so auto tracking cannot touch this repo.
The ordinary automatically linked TODO is sufficient for submission: call
`shape submit APP APPROVED_REV` without `--story` to retain its archived linkage.
An explicit disposable story is optional. In either case inspect the actual
reviewable tracked proposal/run, source identity, verifier evidence and normal
gates; automatic linkage does not waive any submission acceptance criteria.

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
agent changed. Use the opt-in external-console checkpoint below, or an
agent-accessible real browser integration. The checkpoint starts no server and
performs no lifecycle mutations:

```sh
# Run in the pinned umbrella workspace. Use a fresh private directory per observation.
BASHY_E2E_CONSOLE_URL=http://127.0.0.1:22749 \
BASHY_E2E_COOKIE_FILE=/path/to/private/principal-session \
BASHY_E2E_ARTIFACT_DIR=/path/to/private/evidence/before \
BASHY_E2E_APP=hello-world \
BASHY_E2E_PAGE_PATH=/hello-world/ \
BASHY_E2E_GREETING='Hello, World!' \
BASHY_E2E_ACTIVE=FULL_ACTUAL_ACTIVE_SOURCE_COMMIT \
go test ./yoke/pkg/webconsole -tags verifydom \
  -run '^TestDOMExternalShapeCheckpoint$' -count=1 -v
```

Use the actual registered app mount for PAGE_PATH. For preview use
`/shape/hello-world/preview/ACTUAL_CANDIDATE_ID/` and the preview greeting,
while ACTIVE remains the expected live source revision. Run again after
approval and rollback, and with the other principal's session. The cookie file
contains only the paired `bashy_console` value; it is never printed. HTTPS is
required except on loopback. The checkpoint asserts visible `#greeting` text
and the `#shape-active` revision prefix, saves both pages as screenshots and
records browser errors in `checkpoint.json`. Artifacts are private raw
evidence: review before sharing. With no console URL, the test explicitly
skips without launching a browser; that skip establishes no evidence. The
checkpoint alone establishes no real-agent cell PASS.

If the real agent's sandbox permits loopback access but cannot launch Chrome,
the operator may first launch an isolated headless Chrome outside that sandbox
with a disposable profile and a loopback-only remote-debugging listener. Read
its actual `webSocketDebuggerUrl` from the run-scoped browser discovery output;
never use a personal browsing profile or a public listener. Add that private
run reference to the checkpoint command:

```sh
export BASHY_E2E_BROWSER_WS=ws://127.0.0.1:ACTUAL_PORT/devtools/browser/ACTUAL_BROWSER_ID
# Run the same TestDOMExternalShapeCheckpoint command and environment above.
```

Only loopback `ws`/`wss` browser endpoints without credentials, query or fragment
are accepted; do not include tokens. The checkpoint attaches through the
existing chromedp remote allocator and creates a fresh incognito browser
context, disposing it afterward so principals do not share cookies.
Current headless Chrome requires an explicit new window for that incognito
target; the checkpoint creates and disposes this through the existing CDP
connection. Browser failures retain their redacted underlying error in both
the test output and private checkpoint JSON. It leaves the operator's Chrome
process running for the next checkpoint; the operator
owns final process/profile cleanup. The endpoint is a private run reference,
not a new product API. Without this variable, the existing disposable browser
launch path is unchanged. A failed remote connection is a failed checkpoint,
never an automatic fallback to another browser or a passing cell.
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
