# `bashy llm` — the harness-kit primitive

**Status:** design of record for phase P0.5 of
[`plan-agent-harness-positioning.md`](plan-agent-harness-positioning.md). Not
implemented. No code has been written against this document.

**One sentence:** `bashy llm` places one model call — messages and tool
descriptions in, an assistant message and tool calls out — so that an agent loop
becomes something you *write* rather than something you *install*.

---

## 1. Why this verb exists

bashy has most of the reusable substrate and orchestration pieces. The table also names
the planned hardening work; `llm` is the one missing primitive specific to authoring the
loop itself.

| Harness ingredient | bashy today |
|---|---|
| Tool catalog with declared effects | 292 atlas entries, `{read,write,destroy,net,exec,cred,priv,remote,persist,spend}` |
| Guarded dispatch of a shell-resolved external command | the ExecHandler chain — telemetry → audit → execlog → advisor → learn |
| Guarded dispatch of a front-door `bashy <verb>` | **absent** — the top-level dispatcher runs before the ExecHandler chain; P0 must unify the paths |
| Refusal / confinement of a call | P0 policy, P2 confinement |
| Subagents, delegation, gating | the fleet plane — `weave`, `meet`, `judge`, `foreman`, … |
| Procedural + declarative knowledge | `skills`, `craft`, `kb`, `recall`, `graph` |
| Graph execution across tiers | `bashy dag` |
| Durable record | `execlog`, `audit`, P5 |
| A language to write the loop in | Bash++ (designed, unbuilt) |
| **Turning messages into an assistant reply** | **nothing in the tree does this** |

Verified, not assumed: there is no chat-completions client, no `tool_calls`
handling and no provider SDK anywhere in bashy or coreutils. `pkg/ollm` wraps
Ollama for *model management* (list/pull). `pkg/chat` launches third-party agent
CLIs and reads their output — its own telemetry names the turn it observes
`subprocess_harness_turn` and declares `CoverageComplete: false`.

So every model call bashy participates in today is placed by **somebody else's
harness**, which is why bashy can conduct agents but cannot *be* one, and why a
loop cannot be written in Bash++ no matter how good the grammar gets.

This verb is the missing row. It is deliberately **one row**: not a loop, not a
session, not context management.

### 1.1 `invoke` asks an agent; `llm` asks a model

The distinction is the whole scope boundary, and the repo already has the shape
of it (`one-agent-control.md`: *"Invoke is a question, Session is a
conversation"*):

| | asks | returns | owns the loop |
|---|---|---|---|
| `bashy chat` / `invoke` | an **agent** (`tool:model`, a third-party CLI) | whatever that CLI decided to do | the CLI |
| `bashy llm` | a **model** (a provider endpoint) | one assistant message, possibly with tool calls | **the caller** |

`invoke` delegates judgment. `llm` returns judgment and hands it back. A loop is
what the caller writes between the two.

---

## 2. Scope

### In scope

One request → one response. Stateless. Everything the model needs is in the
request; everything the caller gets is in the response.

### Out of scope, permanently

- **No session or conversation store.** The caller owns history. If a loop wants
  to remember, it writes a file — `kb`, `execlog` and the P5 record already exist.
- **No context-window management, no compaction, no summarization.** A loop that
  outgrows its window is the loop's problem, and the loop is a script the user
  can edit. (`dsh` has `ctx.compaction` because it owns the window; bashy does not
  own it and must not pretend to.)
- **No tool execution.** `llm` returns tool calls as *data*. It never runs one.
  See §7 — this is the single most dangerous confusion available here.
- **No agent loop.** Shipping `llm` and then shipping a loop in Go re-creates
  exactly the privileged core that §5.1 of the positioning doc argues against.
- **No model, no inference, no hosting.** A provider seam, not a provider.

---

## 3. Interface

### 3.1 Invocation

```sh
bashy llm [--model NICK] [--tools MODE] [--system FILE] [--max-tokens N]
          [--temperature F] [--timeout DUR] [--dry-run]
```

The request body arrives on **stdin**; the JSON response envelope leaves on
**stdout**. There is no `--message` flag and there will not be one. A model is
required from either `--model` or the request body. Explicit scalar flags override
their body fields; `--system FILE` uses the file's contents. An explicit `tools` array
in the request overrides `--tools`, because replacing caller-authored schemas is not a
safe convenience.

**Why stdin, not argv.** Three reasons, each of which has already bitten
something in this tree:

1. `ps` is world-readable. A prompt in argv is a prompt every user on the box can
   read, and prompts routinely carry file contents, diffs and — despite every
   effort — secrets.
2. bashy's own `audit` and `execlog` planes record **argv**. Putting the
   conversation in argv would silently make every audit record a transcript, at
   0600 if you are lucky, and defeat the `redact` gate that assumes argv is
   short.
3. Messages are structured and arbitrarily large. `ARG_MAX` is not a design.

### 3.2 Request — `bashy-llm-v1`

```json
{
  "schema_version": "bashy-llm-v1",
  "model": "deepseek-v4-pro",
  "system": "You are operating inside bashy…",
  "messages": [
    {"role": "user",      "content": "Make the tests pass."},
    {"role": "assistant", "content": "", "tool_calls": [
      {"id": "c1", "name": "bash", "arguments": {"command": "go test ./..."}}]},
    {"role": "tool",      "tool_call_id": "c1", "content": "FAIL ./pkg/x\n…"}
  ],
  "tools": [
    {"name": "bash",
     "description": "Run a shell command through bashy.",
     "parameters": {"type": "object",
                    "properties": {"command": {"type": "string"}},
                    "required": ["command"]}}
  ],
  "max_tokens": 8192,
  "temperature": 0.0,
  "stop": []
}
```

`model` in the body is optional and **loses to `--model`** when both are present;
a flag is what an operator can see in `ps` and in a policy rule, and the policy
plane must not have to parse stdin to know what is being spent.

### 3.3 Response — `bashy-llm-v1`

```json
{
  "schema_version": "bashy-llm-v1",
  "model":    {"requested": "opus", "resolved": "claude:opus5",
               "upstream_id": "claude-opus-5", "provider": "anthropic",
               "band": 4, "band_source": "measured", "ring": "embedded"},
  "message":  {"role": "assistant", "content": "Running the tests.",
               "tool_calls": [
                 {"id": "c2", "name": "bash",
                  "arguments": {"command": "go test ./pkg/x"}}]},
  "stop_reason": "tool_calls",
  "usage":    {"input_tokens": 4211, "output_tokens": 86,
               "cached_input_tokens": 3900, "source": "provider"},
  "cost":     {"usd": 0.0212, "known": true},
  "duration_ms": 1840,
  "request_id": "…"
}
```

`stop_reason` ∈ `end_turn | tool_calls | max_tokens | stop_sequence | refusal |
content_filter`, normalized across providers. A provider value that does not map
is passed through under `stop_reason_raw` **and** reported as `unknown` — never
coerced into `end_turn`, because "the model finished" and "we did not understand
the provider" are different facts and a loop terminates on the first.

`usage.source` ∈ `provider | estimate`. It is `provider` only when the provider
actually returned counts. This field is why the verb exists at all for
observability: bashy has never once emitted `provider` (§6.2).

`cost.known` is `false` when the model is not in the pricing table, and `usd` is
then **absent**. It is never guessed. `absence-of-evidence.md` records the
precedent: a pricing fallback that billed an unknown model at Claude's rate
produced a plausible number that was not true, and four such numbers nearly got
recorded as facts about a model.

### 3.4 Exit status — borrowed from a seam that got this right

`dsh`'s `ShellExecutor.run()` states the invariant plainly: *"Rejects only for
infrastructure failures… nonzero exits, timeout kills, and abort kills resolve
with a descriptive result."* Same rule here, because a loop needs to distinguish
"the model said no" from "we never reached the model":

| exit | meaning | envelope on stdout |
|---|---|---|
| `0` | the model answered — **including a refusal or a content filter** | yes, complete |
| `1` | infrastructure failure: transport, auth, unresolvable model, malformed request | yes, with `error{}`, no `message` |
| `2` | refused before the call: budget gate, policy deny, `--dry-run` | yes, with `error{}` and `refused_by` |

A loop that treats exit ≠ 0 as fatal is correct. A loop that wants to reason
about a refusal reads `stop_reason`. Both work, which is the point.

---

## 4. Model resolution — the fleet catalog is the registry

No new registry, and no new vocabulary. `--model NICK` resolves through
`pkg/fleet` exactly as `bashy models` and every agent binding already do:

- **Aliases and derived family names.** `opus` is a *derived* alias for the
  highest-versioned member of family `opus`, computed at catalog load. So a
  script pinning `opus` re-points itself on release, and a script pinning
  `opus5` never rots. `bashy llm` inherits this for free and must not
  reimplement it.
- **Ring precedence** — embedded → shared → cloud → local — so an operator's
  local model file beats an org default, unchanged.
- **`Model.Provider`** is the discriminator (`openai` | `anthropic` | `ollama` |
  `a2a` | …). `herald` already established this pattern: a peer agent is an
  ordinary fleet `Model` carrying `provider: a2a` and a `base_url`. `bashy llm`
  adds provider values to a vocabulary that already exists.
- **`Model.UpstreamID`** is the id sent on the wire. Note the existing trap
  recorded in `fleet/types.go`: *"the id a model answers to is a property of the
  TOOL, not of the model."* `bashy llm` is itself a tool, so it reads
  `TargetFor("llm")` and an operator may pin a distinct spelling under
  `ids: {llm: …}` without disturbing ycode's or opencode's.
- **`Model.BaseURL`**, **`Model.ContextLength`**, **`Model.Band`** ride along.

**A model with no reachable provider config is an error, not a fallback.** No
substituting a sibling model, no "closest band", no silent downgrade. The
positioning doc's whole thesis is that a substrate must refuse rather than
improvise; `fleet-live-verification.md` records what improvisation cost last
time — five dead bindings hidden behind one healthy-looking one.

### 4.1 Credentials

`Model.APIKeyRef` names a secret; the value is resolved through `bashy secrets`
(Keychain or the scoped token file), never read from a committed file and never
placed in argv or in a child environment beyond the one process that needs it.
This reuses `agentChildEnv`'s existing rule — *one granted key, scrubbed
environment* — rather than inventing a second credential path.

If the ref does not resolve: exit 1 with `error.dimension = "credential"` and the
**name** of the missing ref. Never the value, and never a guess at an ambient
`OPENAI_API_KEY`. (`agent-launch-needs-vault-env.md` records the inverse failure —
a missing key misread as a stale contract — so the error text must name the ref.)

### 4.2 Providers

T0 ships two, chosen so that the **air-gapped room still runs a loop**:

1. **`ollama`** — via the existing `pkg/ollm`. Local, no key, no external network, no
   spend. This is the one that keeps `philosophy.md` honest: a Bash++ loop
   demonstrably closes on one machine with no account.
2. **`openai`** — the OpenAI-compatible `/chat/completions` shape, which covers
   DeepSeek, z.ai/GLM, Kimi, Groq, Together, vLLM, llama.cpp's server, and any
   pooled gateway, by `base_url` alone. One wire, most of the fleet.

T1 adds **`anthropic`** (the Messages API is a genuinely different wire —
`system` is a top-level parameter, content is a block array, `tool_use` /
`tool_result` rather than `tool_calls`). Sequenced second on purpose: the fleet's
Claude and Codex capacity is reached through **subscription CLIs**, and the
standing cost rule prefers those; a direct metered call to the same vendor is the
expensive path, not the default one.

T2 may add **`a2a`**, so `herald`'s peers answer a plain `llm` call. Nothing in
the design blocks it; nothing needs it yet.

**Refused:** vendoring a fat provider SDK. The OpenAI-compatible and Anthropic
wires are small, stable, JSON-over-HTTPS, and a dependency here is a dependency
in the shell that everything else links. `pkg/acp` sets the precedent — *"this is
the ONE file that imports the SDK"* — and here even that is unnecessary.

---

## 5. Tool schemas — the honest version

The positioning doc said tool schemas could be "generated from the atlas". That
was too glib and this section corrects it.

**The atlas types *effects*, not *arguments*.** `atlas.Entry` is
`{Group, Tier, Stage, Subclass, Caps, Effects, AliasOf}`. `bashy commands X
--features --json` adds `synopsis`, `agent_hint`, `known_gaps`, `available`,
`resolver`. That is a rich *description* of a command and it is **not** a
JSON-Schema of its flags. Nothing in the tree enumerates a command's arguments,
and building that for 292 commands is a project, not a phase.

So `--tools` has three modes, and the default is the one every model is already
trained on:

| mode | what the model sees | when |
|---|---|---|
| `bash` *(default)* | one tool: `bash{command: string}` | almost always |
| `none` | no tools — plain completion | classification, judging, summarizing |
| `+envelope` | `bash`, plus typed tools for the verbs that already emit a stable `--json` envelope (`kb search`, `graph impact`, `todo list`, `skills list`, `commands`) | when a loop wants structured recall without paying a parse |

The atlas earns its keep in two places that are **not** schema generation:

1. **The catalog goes in the system prompt, not the tool list.** A rendered
   digest — what is available on this host, each with its synopsis, effects, and
   `known_gaps` — is a *prompt section*, cheap to cache, and it is how a model
   learns that `grep` here has no back-references before it writes one. That is
   `dsh`'s `ctx.systemPrompt` role, expressed as text bashy already generates.
2. **The atlas keys the policy.** When the loop dispatches the returned command,
   P0 resolves its atlas entry and applies rules by **effect**. The model does
   not need a schema for `rm` to be refused; the substrate does.

`bashy llm` ships a `--catalog` flag that emits that digest (bounded, cacheable,
with a stable ordering so it does not invalidate a KV cache turn to turn) — the
loop pastes it into `system`. Generation of true per-verb JSON-Schema is a
**non-goal of this phase** and probably of every phase.

---

## 6. What the verb plugs into, that already exists

This is not a greenfield verb. Four planes are waiting for a caller.

### 6.1 The budget gate finally sees the truth

`pkg/llmbudget` already has `Check(model, estTokens) Decision` and
`Record(model, prompt, completion, costUSD)`, with per-provider lanes, rate
reservation, and subscription-vs-API-key policy. Today it is fed **estimates**
from subprocess harnesses (`estimateTokens` over a prompt string).

`bashy llm` calls `Check` **before** the request — a `Decision` that is not
`Allowed()` exits 2 with `refused_by: "budget"` and never opens a socket — and
`Record` **after**, with provider-reported counts. The gate stops guessing.

### 6.2 The GenAI telemetry coverage hole closes

`telemetry.GenAICallResult` already carries `UsageSource`, and its test already
exercises the value `"provider"` — which bashy has never emitted, because bashy
has never placed a call. Every span bashy produces today declares
`CoverageScope: "subprocess_harness_turn", CoverageComplete: false`.

A native call emits `CoverageComplete: true` with `UsageSource: "provider"`. That
is the first honest GenAI span in the stack, and it is worth stating plainly:
**this verb is also the fix for an observability gap bashy documented against
itself.**

### 6.3 The output firewall applies to the reply

A model echoes what it was fed. The response passes `pkg/redact` before it
reaches stdout, per `secret-output-firewall.md` — matching **values**, not names.
The request never touches the audit/execlog planes as content (§3.1), so the
firewall has one place to stand.

### 6.4 The call must enter P0's shared governance path

Today `bashy <front-door-verb>` dispatches before the shell's ExecHandler middleware;
adding another `case "llm"` would therefore bypass that chain. P0 must expose one shared
governance entry point for both the front-door dispatcher and shell-resolved external
commands, and `llm` must enter it explicitly. Policy can then refuse the call because its
declared effects include `spend`, and audit can record the invocation. Native GenAI spans
and response redaction are still explicit responsibilities of the `llm` implementation;
the current ExecHandler chain does not provide them automatically.

---

## 7. Security: the returned command is untrusted input

Stated as loudly as the format allows, because it is the one place a reader could
build something dangerous out of a correct design.

**`bashy llm` never executes anything it returns.** It has no exec path. A tool
call in the response is *data the model proposed*, and the proposal came from
text that may include a hostile repository, a poisoned issue, or a web page.

The loop dispatches it — and when it does, that dispatch is an ordinary command
through the ordinary pipeline, where P0 policy, P2 confinement, the audit chain
and the advisor all apply. **That is the entire security argument for this
architecture, and it is stronger than the alternative**: a harness that both
places the call and executes the result inside one process has to invent a
boundary. bashy already has one, and it is a process boundary that predates the
problem.

Two rules that follow, and belong in the shipped skill:

- A loop that pipes `llm` output straight into `eval` has removed the boundary.
  The worked example must not do this, and the docs must say why in the example
  itself, not in a footnote.
- `llm` output is never fed to `bashy run` without the loop deciding. "The model
  asked for it" is not an authorization.

---

## 8. Effects, and the local-first contract

Atlas entry:

```
llm   userland/orchestration  [json, needs-network]  {net, spend}
```

- `{net, spend}` places it beside `judge`, `meet`, `delegate` and `chat`, which
  already declare exactly this.
- It is **not** added to the local-first lifecycle list in
  `pkg/atlas/localfirst_test.go`. The enforced claim — *no verb in the
  todo/sprint → weave → gate/check → dag loop declares `net`* — is untouched. The
  static effects say what the verb may do, not that every provider does it. With
  `provider: ollama`, the same inference-driven loop runs against the local runtime in
  the air-gapped room, matching `philosophy.md` §3's local answer for inference.
- **A test must assert the absence**, not merely fail to add it. `llm` appearing
  in the loop list should break the build, in a diff, where a reviewer can ask
  what it bought.
- **The precedent is already in that file, and it is stronger than expected.**
  `TestInferenceHasALocalAnswer` asserts that `ollama` remains a verb bashy can
  launch, on the reasoning that *"judge, invoke and meet need a model… unless the
  model runs here too… the prerequisite is in the box."* `bashy llm` joins
  exactly that set and must be added to that test's verb list, which means the
  local-first claim for this verb is not a promise in prose — it is the same
  assertion that already guards `judge`, `invoke` and `pair`. This is also why
  §4.2 ships the `ollama` provider **first**: the test's premise is that the
  local answer exists, and a `net`-only `llm` would be the first LLM-shaped verb
  to falsify it.
- `provider: ollama` reaches `127.0.0.1`. It still declares `net` — an effect
  describes the *capability exercised*, not the *destination reached*, and
  weakening that for a loopback case is how effect vocabularies rot.

---

## 9. Record / replay — how the gate runs offline

Borrowed directly from `dsh`, which ships `llm-replay` as a provider beside its
real ones. Two environment variables, no new verb:

- `BASHY_LLM_RECORD=DIR` — write each request/response pair, content-addressed by
  a hash of the **normalized request** (messages, tools, model, sampling
  parameters), redacted on the way in.
- `BASHY_LLM_REPLAY=DIR` — serve from the cassette; **a miss is exit 1**, never a
  live call. Silent fallthrough to the network is the failure mode that makes a
  replay suite meaningless.

This is what makes P0.5's acceptance gate (§11) runnable in CI, on a plane, with
no key and no spend — which is the only way a local-first project is allowed to
have a test for a `net` verb.

---

## 10. What a loop looks like

Bash++ is unbuilt, so the design is shown in **both** the language it targets and
the bash it must work in today. If it does not work in plain bash, the verb is
wrong.

**Today, plain bash + `jq`** (the worked example that ships with P0.5):

```bash
#!/usr/bin/env bashy
msgs=$(jq -n --arg t "$1" '[{role:"user",content:$t}]')
sys=$(bashy llm --catalog)

for step in $(seq 1 40); do
  resp=$(jq -n --argjson m "$msgs" --arg s "$sys" \
    '{schema_version:"bashy-llm-v1",system:$s,messages:$m}' |
    bashy llm --model "$MODEL" --tools bash) || exit 1

  msgs=$(jq --argjson r "$(jq .message <<<"$resp")" '. + [$r]' <<<"$msgs")
  [ "$(jq -r .stop_reason <<<"$resp")" = tool_calls ] || break

  while read -r call; do
    id=$(jq -r .id <<<"$call")
    # Re-enter the AgentOS shell. `bash -c` is the pure drop-in and would bypass
    # bashy's governance middleware for commands inside the script.
    out=$(bashy run --capture -- bashy -c "$(jq -r .arguments.command <<<"$call")")
    msgs=$(jq --arg i "$id" --arg o "$out" \
      '. + [{role:"tool",tool_call_id:$i,content:$o}]' <<<"$msgs")
  done < <(jq -c '.message.tool_calls[]?' <<<"$resp")
done

bashy gate            # the verdict, not the model's opinion of the verdict
```

**With Bash++ (P6), the same loop with the parts that hurt removed** —
`jq` round-trips become values, and the fan-out becomes structure:

```go
msgs := []Message{{role: "user", content: task}}
defer bashy_gate_report()

for step := range 40 {
    resp, err := llm(msgs, catalog)
    if err != nil { return 1 }
    msgs = append(msgs, resp.message)
    if resp.stop_reason != "tool_calls" { break }

    results := make(chan ToolResult, 8)
    for _, call := range resp.message.tool_calls {
        go dispatch(call, results)          // real processes, which is the point
    }
    for range resp.message.tool_calls {
        msgs = append(msgs, <-results)
    }
}
```

Two things this comparison is meant to prove, and they are the reason the phases
are ordered P0.5 → P6 and not the reverse:

1. **`llm` is useful before Bash++ exists.** The bash version works, today,
   with `jq`.
2. **Bash++ is what makes it pleasant, and the concurrency is not decoration.**
   Fanning out tool calls to real processes is the thing a shell was always for,
   and it is the row where a Bash++ loop is *better* than a TypeScript one rather
   than merely equivalent.

---

## 11. Acceptance gate

The phase is done when **all** of these hold:

1. A loop authored as a script — not compiled into bashy — drives a real
   repository task to a **green `bashy gate`**, with the whole loop readable and
   editable by the user.
2. The same loop runs **offline** under `BASHY_LLM_REPLAY`, in CI, with no key
   and no spend, and a cassette miss fails the run.
3. The same loop runs **against a local Ollama model with no external network**, proving
   the air-gapped claim for an inference-driven loop.
4. `usage.source == "provider"` and the emitted GenAI span carries
   `CoverageComplete: true` — the first in the stack.
5. `llmbudget` records real counts; a `Check` refusal exits 2 **without opening a
   socket** (assert on the socket, not on the exit code).
6. An unknown model produces `cost.known: false` **with no `usd` field** — the
   `absence-of-evidence` regression test.
7. `pkg/atlas/localfirst_test.go` fails if `llm` is added to the lifecycle loop
   list.
8. Existing gates unchanged: `make test-bash` 86/86, `go test ./...`, the
   Windows cross-build, and the e2e dispatch gate (which will fail on day one if
   `llm` ships without its atlas entry — that is the mechanism working).

**The gate that decides the thesis is #1.** Items 2–8 make it trustworthy;
item 1 is what the positioning document is actually claiming, and until it runs,
the claim is a design. `plan-agent-harness-positioning.md` §7 item 5 records the
split falsification condition: failure of the plain-bash loop rejects the harness-kit
thesis; failure only of its later Bash++ port rejects the Bash++ authoring thesis.

---

## 12. Open questions

Recorded rather than resolved, because each changes an interface and none blocks
starting.

1. **Streaming.** T0 returns a complete message; a loop consumes one. `--stream`
   emitting NDJSON deltas is easy to add and impossible to remove. Defer until a
   caller needs it — a TUI would, and bashy is not shipping one.
2. **Prompt caching.** The `--catalog` digest is the obvious cacheable prefix and
   the providers expose cache controls differently. Stable ordering (§5) is
   enough for T0; explicit cache-breakpoint control is T1, and `usage` already
   has `cached_input_tokens` reserved for the reporting half.
3. **Multi-tool parallel calls.** Providers differ on whether tool calls in one
   message may run concurrently. The bash example serializes; the Bash++ example
   fans out. Whether `llm` should *report* a provider's parallelism hint, or
   leave it to the loop, is unresolved — leaning to leave it, since the loop is
   what knows whether two commands touch the same file.
4. **`--tools +envelope` membership.** Which `--json` verbs earn a typed schema
   is a curation question that should be answered by a loop that wanted one, not
   in advance.
5. **Images and attachments.** Out of scope for T0. When it lands it is a content
   part in a message, not a new flag, and `pkg/attachment`-shaped storage is a
   separate decision.
6. **Does `chat`/`invoke` gain an `--llm` backend?** Once `llm` exists, a
   `tool: llm` fleet entry would make "the model itself" an agent in the roster,
   selectable by band like any other. Attractive and premature: it changes what
   `agents verify --live` means. Revisit after the gate.

---

## 13. Refused, with reasons

| Proposal | Why not |
|---|---|
| A session/conversation store in `llm` | The caller owns history. A store here grows a loop around it, and then bashy has an opinion. |
| Context compaction / auto-summarize | bashy does not own the window. A loop that needs this can call `llm` again with a summarize prompt — visibly, in the script. |
| Executing returned tool calls | §7. There is no exec path, and adding one collapses the process boundary that makes this architecture defensible. |
| Retry-with-backoff policy beyond one transport retry | Retry is a loop decision. A hidden retry doubles a bill and hides a provider outage. |
| Falling back to another model on failure | `fleet-live-verification.md`: a substitution that looks healthy is worse than a refusal that is loud. |
| Guessing cost for an unpriced model | `absence-of-evidence.md`, precedent set. `known: false`, no number. |
| Vendoring a provider SDK | Two small JSON wires versus a dependency in the binary every other verb links. |
| Reading `OPENAI_API_KEY` from the ambient environment | Credentials come from the declared `api_key_ref` through `secrets`. An ambient key is an unaudited grant, and it makes "which key paid for this" unanswerable. |
| Per-verb JSON-Schema generation from the atlas | §5. The atlas types effects, not arguments. Claiming otherwise would ship 292 schemas that describe nothing. |
| `bashy llm` in the local-first loop list | §8. It would be the first `net` verb in the loop and the enforcement test exists precisely to make that a visible trade. |
