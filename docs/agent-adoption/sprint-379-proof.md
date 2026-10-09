# Agent shell proof and output recovery — Sprint 379, Story 1527

Recorded 2026-10-08; follow-up 2026-10-09 on macOS arm64. This is a bounded delivery record, not
an assertion that every agent passed. No agent was removed from scope.
The earlier Claude/OpenCode evidence remains in [matrix.md](matrix.md).

## Plan and acceptance

1. Run installed Codex and AGY first; install Gemini/Copilot without a paid
   signup; exercise Aider already installed on the host.
2. Wire through `install-agent`, give the real CLI a one-turn shell task,
   and require a successful **bashy execution-log record**, not a model's
   assertion that it used bashy. Record exact external blockers.
3. Reproduce routing and discovery defects with failing tests; fix the
   shared shim writer; graduate `out` only with recovery/error coverage.
4. Run module builds/vet and focused tests, then commit in each repository.

## Results and remaining work (2026-10-08 baseline)

| CLI | Version | Observed result | Remaining limit/blocker |
|---|---|---|---|
| Codex | 0.157.1 | **PASS with explicit shell-tool selection** of the wrapper written by `install-agent codex`; real model turn, observed successful `seq` episode below | Default shell remains `/bin/zsh`: PATH/SHELL alone did not route it. The account-wide `chsh` recipe was not applied. This is not a default-routing pass. |
| AGY | 1.3.1 | **BLOCKED before tool execution**, CLI exit 3 | `RESOURCE_EXHAUSTED` HTTP 429: individual subscription quota reached; reset reported in 3h1m21s. No execution episode. Retry after quota reset; no upgrade purchased. |
| Gemini | 0.63.0 | Package installed; **BLOCKED at authentication**, exit 1 | `IneligibleTierError`, `UNSUPPORTED_CLIENT`, `free-tier`: server says Gemini Code Assist for individuals no longer supports this client and directs migration to Antigravity. Reproduced after resolving workspace trust. No execution episode. |
| Aider | 0.86.2 | **PASS for real CLI `/run` under a PTY**, successful bashy episode below | This is a deterministic CLI command, **not a model-driven turn**. Piped stdin uses `/bin/sh` despite `$SHELL`; installed source confirms that behavior. An LLM turn remains unproven: local Ollama connection refused, and existing metered-provider credentials were not substituted for subscription access. |
| Copilot | 1.0.93 | Package installed; **BLOCKED at authentication**, exit 1 | Inherited classic PAT rejected. With `GITHUB_TOKEN` and `GH_TOKEN` removed, CLI reports no authentication information. Needs OAuth login or a supported fine-grained token; no execution episode. |

Y6's full live-agent acceptance remains open for these limitations. In
particular, shim unit/e2e tests are not counted as AGY/Gemini/Copilot model
turns, and Aider's `/run` is not counted as an LLM turn.

The 2026-10-09 follow-up below supersedes AGY's blocked verdict and adds
fresh evidence for every remaining client. Default Codex routing is still open.

## Reproduction and transcripts

The same pattern as the Claude/OpenCode proof is used: install a shell entry
point, run the real CLI, and inspect the shell-side execution evidence.
A private temporary install HOME keeps account profiles and login shells
unchanged; the real agent HOME retains existing subscription authentication.

```sh
# BASHY is the absolute candidate binary; PROOF is a fresh absolute directory.
mkdir -p "$PROOF/install-home"
HOME="$PROOF/install-home" "$BASHY" install-agent codex --shell "$BASHY"
export PATH="$PROOF/install-home/.bashy/shims:$PATH"
export SHELL="$BASHY"
export BASHY_AGENTIC=1 BASHY_OUTPUT_REDUCE=off
export BASHY_EXECHIST="$PROOF/exec"
```

Each agent used its own fresh proof directory. API-key variables and inherited
parent session markers were removed for these subscription probes. The prompt
for the unqualified/default attempts was:

> Use your shell command tool to run exactly `seq 3791527 3791527`. Do not read files, use other tools, or make changes. Report only its stdout.

Codex default attempt (exit 0, **zero** matching bashy episodes):

```text
codex exec --ignore-user-config --skip-git-repo-check --ephemeral --json \
  -s danger-full-access '<prompt>'
command_execution: /bin/zsh -lc 'seq 3791527 3791527'
```

Codex positive attempt used that same invocation, with this exact prompt
(the path is the wrapper produced by `install-agent`, not a second shell
started inside the command):

```text
Use exec_command with shell="/tmp/s379-proof/codex/install-home/.bashy/shims/bash" and login=false to run exactly `seq 3791527 3791527`. Do not read files, use other tools, or make changes. Report only its stdout.
```

Recorded CLI events, reduced to relevant fields:

```json
{"type":"command_execution","command":"/tmp/s379-proof/codex/install-home/.bashy/shims/bash -c 'seq 3791527 3791527'","aggregated_output":"3791527\n","exit_code":0,"status":"completed"}
```

Independent bashy record (only cwd omitted):

```json
{"schema":"bashy-execlog-v1","canon_ver":1,"stage":"episode","at":"2026-10-08T17:37:30.733472Z","episode":"ep-74c67f1fdad04d7c","pid":34319,"ppid":34006,"seq":1,"cmd":"seq","template":"seq <N> <N>","argv":["seq","3791527","3791527"],"exit":0,"observed":true,"duration_ms":0,"effects":["pure"],"redaction":{"scrubber":"redact/1","n":0}}
```

Aider used `install-agent aider --shell "$BASHY"`, then its emitted `SHELL=`
recipe. An empty YAML mapping (`{}`) supplied a separate config file. With
stdin attached to a PTY:

```sh
aider --config "$PROOF/config.yml" --env-file /dev/null --no-git \
  --no-check-update --no-analytics --yes-always \
  --model ollama_chat/qwen2.5-coder:7b --message '/run seq 3791527 3791527'
```

```text
OllamaError: Error getting model info ... [Errno 61] Connection refused
Aider v0.86.2
3791527
Added 2 lines of output to the chat.
CLI exit: 0
```

Independent bashy record (only cwd omitted):

```json
{"schema":"bashy-execlog-v1","canon_ver":1,"stage":"episode","at":"2026-10-08T17:40:12.801779Z","episode":"ep-f0d370fc0f5b2b1d","pid":43945,"ppid":43934,"seq":1,"cmd":"seq","template":"seq <N> <N>","argv":["seq","3791527","3791527"],"exit":0,"observed":true,"duration_ms":0,"effects":["pure"],"redaction":{"scrubber":"redact/1","n":0}}
```

With piped stdin, Aider exited 0 but generated no `seq` episode. In Aider
0.86.2, `aider/run_cmd.py` chooses pexpect only for a TTY; the alternative
uses `subprocess.Popen(shell=True)` without `executable=`, ignoring the
`SHELL` value it reads. Bashy's installer now calls out that limitation.

The blocked invocations, after each corresponding `install-agent`:

```sh
agy --print '<prompt>' --print-timeout 90s --output-format stream-json \
  --dangerously-skip-permissions
# exit 3: Individual quota reached. ... Resets in 3h1m21s.

npm install --prefix "$PROOF/cli" @google/gemini-cli@0.63.0 @github/copilot@1.0.93
# exit 0; both binaries report the versions above

GEMINI_CLI_TRUST_WORKSPACE=true gemini -p '<prompt>' \
  --approval-mode yolo --output-format stream-json
# exit 1: IneligibleTierError / UNSUPPORTED_CLIENT / free-tier

copilot -p '<prompt>' --allow-all-tools --no-ask-user
# exit 1 with classic PAT; retry without GITHUB_TOKEN/GH_TOKEN:
# Error: No authentication information found.
```

All launches had a 120-second outer deadline. None of the blockers are a
shell timeout. CLI output and `$0` alone are not proof; the old `--probe`
response-text heuristic was not used as an acceptance gate.

## Root cause and fix

The multicall product intentionally treats invocation names `bash` and `sh`
as plain shell entry points. The old installer and managed launcher created
symlinks with those names. Commands ran, but bypassed AgentOS and its exec
middleware. A successful `echo ok` shape check could not expose the loss.

Both paths now call yoke's `chat.WriteShellShim`, which atomically replaces
legacy symlinks with a small POSIX wrapper that execs the absolute bashy path
with unchanged arguments. Quoting covers spaces and apostrophes; migration
never writes through the legacy symlink into the binary. Plain shell aliases
outside the agent shim directory keep their existing behavior.

## `out` graduation and usage

The maturity flag was bashy's `curatedHiddenVerbs` entry, projected into the
atlas as `hidden:true, status:"experimental"`. `out` now appears in the
default catalog without that flag. Its execution tier remains **userland**;
classification remains diagnostics/cross, read-only, read effect, portable.
The shell-owned `out` atlas entry lives in bashy, not yoke's shared table.

```sh
BASHY_OUTPUT_REDUCE=on bashy -c 'seq 1 20000'
# Copy the exact handle from the emitted recovery marker:
bashy out HANDLE
bashy out HANDLE > complete.txt
bashy out HANDLE | rg '19999'
```

Recovery accepts an unambiguous hexadecimal prefix, full hexadecimal digest,
or `sha256:` digest. Artifacts live at `$BASHY_HOME/exec/output`, defaulting
to `~/.bashy/exec/output`; recover under the same store root as the producer.
It returns the complete **stored** bytes, after secret redaction and display
normalization. It cannot recover the original secret or a deleted artifact.
Binary bytes and a missing trailing newline remain exact. Wrong arity,
invalid/missing/ambiguous handles, store failures, and output-write failures
return errors; shortening an ambiguous prefix is never guessed.

`out` does not enable reduction. Stage 1 remains explicit opt-in;
`BASHY_OUTPUT_REDUCE=off`, `--no-elide`, and `--full` keep complete output.

## Validation

Bases: bashy `039ebd6`, yoke `6103baf`, plus this delivery's changes in the
sibling Go workspace. The live proof binary was built with
`scripts/go-product.sh build -o "$BASHY" ./cmd/bashy`; SHA-256:
`ca4ce23af1a22e088e4a8f61b136b2eff593496a71cb471ac7b10899daa84d5c`.
Later changes clarify help/docs and extend tests; shell routing is unchanged.

Red first (each exit 1):

- bashy: `go test ./internal/agentos -run 'Test(InstallAgentShimPreservesAgentOSEntryPoint|OutGraduatedInAtlasAndDefaultCatalog)$' -count=1` — all four installer routes lost target identity; `out` was hidden/experimental.
- yoke: `go test ./pkg/chat -run '^TestEnsureShimsPreservesAgentOSEntryPoint$' -count=1` — all three shell names lost target identity.

Green, focused on the development host (exit 0):

- Both modules: `go build ./...` and `go vet ./...`, individually captured exit codes, no pipeline gate.
- bashy: `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/s379-proof/bashy-windows.exe ./cmd/bashy` (compile only, exit 0).
- yoke: `go test ./pkg/reduce -count=1` (entire small recovery package, exit 0).
- yoke: `/bin/sh scripts/crossvet.sh` (Windows/Linux/macOS vet, AIX compile canary, applet coverage; exit 0).
- bashy: `go test ./internal/agentos -run 'Test(InstallAgentShimPreservesAgentOSEntryPoint|OutGraduatedInAtlasAndDefaultCatalog|Commands|Atlas|ClassSections|OutputReduction|OutputShape|ShellOutput|DryRunAndReducer)' -count=1`.
- yoke: `go test ./pkg/reduce ./pkg/chat -run 'Test(OutCommandContract|RecoverViaOutCmd|Store|EnsureShims|ForcedShell)' -count=1`.

The e2e test executable was built on the development host with
`go test -c -tags e2e -o agentos.test ./internal/agentos` and run against the
candidate binary on the separate macOS arm64 test host, with
`BASHY_E2E_BIN` set and a 120-second timeout. All passed, exit 0:

- `TestE2EInstallAgentShimRecordsExecution` (codex, agy, gemini, copilot).
- `TestE2EOutputReductionCoreutilsIsBoundedDeterministicAndRecoverable`.
- `TestE2EOutputReductionExternalStreamsRedactBeforeSpill`.
- `TestE2EOutputReductionRollbackAndDataPathsStayExact` (including data sinks).

No full suite ran on the development host. No push or CI run was requested
under the worker contract; no CI URL exists for this delivery. Integration
must publish yoke `2e89c45` first and update bashy's yoke module pin to include the
shared shim writer before standalone CI, then bump umbrella pins.

## Follow-up: 2026-10-09 (same story, partial delivery)

No client was dropped. The current verdicts are:

| Client | Verdict | Evidence / remaining work |
|---|---|---|
| Codex 0.157.1 | **FAIL: default routing** | Two fresh model turns, without `--ignore-user-config`, still execute `/bin/zsh`. Setting `shell_environment_policy.set.SHELL` also leaves the executable unchanged. Zero matching bashy episodes. No default-routing fix or generated-config regression is delivered. |
| AGY 1.3.1 | **PASS: model-driven shell execution** | `gemini-3.1-pro-high` chose `run_command`, returned the expected integer, and bashy independently recorded exit 0. The earlier quota blocker did not recur; no ten-minute quota retry was needed. |
| Gemini 0.63.0 | **BLOCKED: client eligibility** | Fresh authenticated attempt still returns `IneligibleTierError` / `UNSUPPORTED_CLIENT` / `free-tier`, exit 1. AI Studio has a documented free API-key path, but no Gemini/Google API key is available in this process. |
| Copilot 1.0.93 | **BLOCKED: authentication** | Fresh attempt with inherited GitHub tokens removed returns `No authentication information found.`, exit 1. A free plan exists; browser/device OAuth or a supported token is still needed. |
| Aider 0.86.2 | **BLOCKED: model provider adapter** | A real model-driven PTY attempt using installed LiteLLM's `chatgpt/gpt-5.2` provider fails with `ChatgptException - argument of type 'NoneType' is not iterable`. Zero matching bashy episodes. Earlier deterministic `/run` PASS remains valid but is not a model-driven PASS. |

### Codex: root-cause boundary, not a generated-config fix

The installer lives in **bashy**, `internal/agentos/installagent.go`, not in
yoke. `codexInstaller` writes a bash-named exec wrapper and prints a `chsh`
recipe. It does not generate `config.toml` or change the account shell by
default. There is therefore no generated Codex shell configuration to test.
The prior `--ignore-user-config` was a confounder, but removing it does not
repair this behavior.

The upstream selection code uses `getpwuid_r(...).pw_shell`, derives its type,
and prefers that existing account-shell executable over PATH. It does not
consult `$SHELL` to select the executable. See
[Codex shell detection source](https://github.com/openai/codex/blob/main/codex-rs/shell-command/src/shell_detect.rs)
(`default_user_shell`, `get_shell_path`) and the
[configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference).
This source was inspected on the follow-up date; the installed CLI behavior
below is the direct evidence for version 0.157.1.

The installed parser independently rejects the candidate top-level setting:

```sh
codex app-server --strict-config -c 'shell="/tmp/s379-followup/codex/install-home/.bashy/shims/bash"' --listen off
# exit 1
# Error: unknown configuration field `shell` in -c/--config override
```

Fresh isolated install HOME, isolated CODEX_HOME using the existing account's
subscription authentication, PATH beginning with the generated shim directory,
`SHELL` pointing at bashy, `BASHY_AGENTIC=1`, reduction off, and a fresh
`BASHY_EXECHIST` store. The first run had no config file; the second used:

```toml
[shell_environment_policy.set]
SHELL = "/tmp/s379-followup/codex/install-home/.bashy/shims/bash"
```

Both used the original unqualified prompt from this document:

```sh
codex exec --skip-git-repo-check --ephemeral --json \
  -s danger-full-access '<prompt>'
```

Both returned exit 0 and this completed execution event (projected fields):

```json
{"type":"command_execution","command":"/bin/zsh -lc 'seq 3791527 3791527'","aggregated_output":"3.79153e+06\n3.79153e+06\n","exit_code":0,"status":"completed"}
```

No matching bashy `seq` episode was present. The CLI's exit 0 and its reported
output are not acceptance. No account-wide `chsh` was performed. This request
remains blocked on a supported upstream default-shell override (or a separately
chosen account-shell deployment). An ignored config key, a prompt asking for
an explicit shell, or a fabricated config-unit PASS would not fix the root.
There is **no yoke patch** in this delivery.

### AGY: fresh end-to-end PASS

After `bashy install-agent agy --shell "$BASHY"`, prepend its emitted shim
directory to PATH, retain the real authenticated HOME, and set the same
execution-log environment described above:

```sh
agy --print '<prompt>' --model gemini-3.1-pro-high \
  --log-file "$PROOF/agy.log" --print-timeout 90s \
  --output-format stream-json --dangerously-skip-permissions
# exit 0
```

Relevant stream events (conversation identifiers omitted):

```json
{"step_type":"tool","state":"DONE","tool_name":"run_command","tool_info":{"name":"run_command","parameters":{"CommandLine":"seq 3791527 3791527"},"output":"3791527\r\n"}}
{"event":"result","result":{"status":"SUCCESS","response":"3791527\n","num_turns":1}}
```

Independent execution-log record (only cwd omitted):

```json
{"schema":"bashy-execlog-v1","canon_ver":1,"stage":"episode","at":"2026-10-09T09:10:37.070678Z","episode":"ep-3ef111c3a271b1fc","pid":46302,"ppid":44891,"seq":1,"cmd":"seq","template":"seq <N> <N>","argv":["seq","3791527","3791527"],"exit":0,"observed":true,"duration_ms":0,"effects":["pure"],"redaction":{"scrubber":"redact/1","n":0}}
```

A preceding default-model attempt also completed with exit 0 and a successful
bashy episode at `2026-10-09T09:09:49.558722Z`; the explicit-model rerun above
makes the model selection reproducible. Initial `error_message` stream events
alone were not treated as a final verdict. Neither completed attempt returned
the prior HTTP 429 quota error.

### Gemini and Copilot: exact fresh blockers and free paths

With the generated shim directory at the front of PATH and the original
unqualified prompt, Gemini still fails before shell execution:

```sh
GEMINI_CLI_TRUST_WORKSPACE=true gemini -p '<prompt>' \
  --approval-mode yolo --output-format stream-json
# exit 1; zero matching bashy episodes
```

```text
Error authenticating: IneligibleTierError: This client is no longer supported for Gemini Code Assist for individuals. To continue using Gemini, please migrate to the Antigravity suite of products: https://antigravity.google
reasonCode: 'UNSUPPORTED_CLIENT'
tierId: 'free-tier'
```

The CLI's [authentication guide](https://geminicli.com/docs/get-started/authentication/)
and [plans](https://geminicli.com/plans/) document an AI Studio API-key free tier.
This is a potential non-paid alternative, not a proven usable account on this
host. A presence-only environment check reported `GEMINI_API_KEY: absent` and
`GOOGLE_API_KEY: absent`; no credential values were printed. The authenticated
consumer path above remains ineligible.

```sh
env -u GITHUB_TOKEN -u GH_TOKEN copilot -p '<prompt>' \
  --allow-all-tools --no-ask-user
# exit 1; zero matching bashy episodes
```

```text
Error: No authentication information found.

Copilot can be authenticated with GitHub using an OAuth Token or a Fine-Grained Personal Access Token.

To authenticate, you can use any of the following methods:
  • Start 'copilot' and run the '/login' command
  • Set the COPILOT_GITHUB_TOKEN, GH_TOKEN, or GITHUB_TOKEN environment variable
  • Run 'gh auth login' to authenticate with the GitHub CLI
```

GitHub documents [Copilot Free](https://docs.github.com/en/copilot/how-tos/manage-your-account/get-started-with-a-copilot-plan)
and [CLI OAuth authentication](https://docs.github.com/en/copilot/how-tos/copilot-cli/set-up-copilot-cli/authenticate-copilot-cli).
The CLI is available on all plans. The blocker is a usable login, not a proven
requirement to buy a subscription. No purchase, signup, or browser authorization
was completed by this worker.

### Aider: subscription-backed model attempt, no shell proof

The installed LiteLLM 1.81.10 includes a `chatgpt` OAuth provider. This attempt used a
private mode-0600 temporary auth record containing the existing Codex account's
access/identity tokens, **no refresh token** and no metered API key. The private
copy was removed after the attempt. Nothing from the auth record is in this
document or commit.

With a PTY, `CHATGPT_TOKEN_DIR` pointing at that private directory, `SHELL`
pointing at bashy as emitted by `install-agent aider`, an empty YAML config,
`BASHY_AGENTIC=1`, and a fresh execution-log store:

```sh
aider --config "$PROOF/config.yml" --env-file /dev/null --no-git \
  --no-check-update --no-analytics --yes-always --model chatgpt/gpt-5.2 \
  --message 'Choose and run a shell command that prints the one-integer sequence from 3791527 through 3791527. Use your shell command facility; do not edit files. Report the actual command output.'
```

```text
Aider v0.86.2
Main model: chatgpt/gpt-5.2 with whole edit format
litellm.APIConnectionError: APIConnectionError: ChatgptException - argument of
 type 'NoneType' is not iterable
Retrying in 0.2 seconds...
```

The same error recurred through the displayed 32-second retry delay. No model
answer or shell-command proposal arrived, and no matching bashy episode was
recorded. The outer PTY harness reached its 120-second cap; its process-group
termination raised `PermissionError: [Errno 1] Operation not permitted`, so no
reliable client exit code was captured. The PTY closed and the child was absent
from the subsequent process check. This is **not** an exit-0 or routing PASS.
Diagnosing the external Aider/LiteLLM adapter remains outside this bashy-only
worker's patch scope.

### Follow-up validation

The follow-up changes documentation only; no production fix, red/green unit
regression, generated-config test, or new yoke patch is claimed. The existing
proof binary reports `de929cc`, matching this workspace's `de929cc9` base.
Live client probes ran on the authenticated development host; no conformance
suite or full unit suite ran there.

All commands below ran on the development host with Go 1.27.1, captured to
separate log files with `rc=$?` immediately after the command (no pipeline gate):

- `go build ./...`: first attempt exit 143 with an empty diagnostic log.
- `GOFLAGS=-p=2 go build ./...`: bounded-parallelism retry **exit 0**.
- `go vet ./internal/agentos`: **exit 0**.
- `go test ./internal/agentos -run 'Test(InstallAgentShimPreservesAgentOSEntryPoint|OutGraduatedInAtlasAndDefaultCatalog)$' -count=1`: **exit 0**, package result `ok`, 1.458s. Both named tests exist; neither is skipped.
- `git diff --check`: **exit 0**.

The successful Go build/vet logs were empty, and the focused-test log contained
the successful package result. These checks preserve the existing implementation;
they do not turn any blocked live-client verdict into PASS.

No push or CI run was performed. Story 1527 remains open for default Codex
routing and the three blocked client proofs above.
