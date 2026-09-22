# bashy docs index

One line (or a short paragraph) per load-bearing doc under `docs/`, moved verbatim from `CLAUDE.md` §Doc index on 2026-09-20. `CLAUDE.md` only points here; when you add a doc, add its entry here.


- `contracts.md` — **Bash++ contracts (design by contract), Sprint 203:**
  `Require:`/`Ensure:`/`Effects:` on a dag target, `@require`/`@ensure`/`@guard`
  on a function; require → body → ensure, exit 3 naming the clause, a check is a
  SHELL COMMAND run in the call's own frame via `Call.Run` (`$1..$n`, the
  callee's variables, `STATUS`/`RESULT`; assignments discarded — single-quote
  it). An
  agentic yield (exit 6) propagates with no `ensure` run; the fixture is
  `test/contracts/` (`run.sh [bashy]` replays it on any binary). Natives live in
  `internal/agentos/contracts.go`, source-only like `@retry`.
- `function-attestation.md` — **Sprint 216 B18:** a decorated Bash# function
  or `agentic function` completing appends ONE `skills.AttestRecord` to the
  EXISTING yoke skills/craft ledger (`<store>/attest/<name>.jsonl`, read back by
  `bashy craft history`) — clause verdicts as `<clause>:<check>`, exit status,
  coordinate, `bashy@<version>` tier, store revision. A yield (exit 6) is a
  HANDOFF (`status: 6`, `valid: false`, counted `yielded`, never `FAIL`). The
  outermost native rung on a call appends, inner rungs contribute; advice puts
  the pass-through `attest` rung on every agentic registration. `BASHY_ATTEST=0`
  off; Bash OFF / `--posix` / `cmd/bash` structurally silent. The yoke seam
  (`AppendAttest`, `Status`, `StoreRevision`, `Stats.Yielded`) is pinned there.
- `effect-derived-confirmation.md` — **`@confirm` (B16, Sprint 216):** PowerShell's
  `SupportsShouldProcess` without the shell. A Bash# function declared `@confirm()`
  gains a generated `--what-if` (describe every governed operation, run none) and a
  per-operation confirmation of high-impact operations (declared `destroy`/`spend`/
  `cred`/`priv`, or unclassified — fail closed), derived from the Command Atlas /
  registered-command effects only, decided at the same ExecHandler rung as the `@guard`
  cap. The human is reached through `bashy ask`; when no channel reaches one the call
  YIELDS exit 6 with the resume form `--confirm=TOKEN:yes|no` (token = the operation's
  argv), the contract `rfcs/0001-agentic-yield.md` (B19) pins: a harness pauses, asks,
  and replays — never a fabricated yes/no. `test/yield/run.sh` is the recorded
  harness fixture. Bash OFF is identity.
- `agentic-action-example.md` — Sprint 134's bare `agentic` action examples:
  typed function/method, shell function, executable script and native tool
  embedding through the existing handler context and governed chat invocation.
  `tools/agentic-example` is an example binary, not a new standard bashy verb.

- `philosophy.md` — **the thesis: LOCAL FIRST.** "bashy is all an agent needs" — the whole
  SDLC loop (issue → weave → gate → judge → dag) closes on ONE machine with NO network,
  and that claim is *enforced*, not asserted: `pkg/atlas/localfirst_test.go` fails the
  build if a loop verb starts declaring the `net` effect. The air-gapped room is a TEST,
  not a market (if it works there it works on the plane, in the outage, and on hotel
  wifi). Three pillars (compatibility → capability → agency — a DIFFERENT axis from the
  three substrates in §Name; pillars I+II both land in Classic), six venues (venue 1 is a
  complete product, not a fallback), and what the philosophy FORBIDS. Read before any
  feature that reaches for a hosted service.
- `TODO.md` — phase checklist + current PASS/FAIL/SKIP headline. Always read first.
- `report-bash53-test-status.md` — per-fixture status snapshot from the bash 5.3 suite.
- `sprint-253-windows-locale-service-blocker.md` — Story #711's fail-closed
  Windows host-locale handoff and exact non-completion evidence: MSYS/Cygwin
  service six corpus encodings but not Big5-HKSCS; Windows ICU has that
  converter but not the required POSIX `LC_MESSAGES` category.
- `handoff-bashy-2026-06.md` — most recent session-handoff notes (read when picking up cold).
- `bash-gap-analysis.md` — ungated bash semantics gap analysis behind the failing fixtures.
- `plan-bashy-drop-in.md` / `plan-cmd-bashy.md` / `plan-bash53-roadmap-agentic.md` — phase plans; each phase lands as a checkbox in `TODO.md`.
- `followup-signal-death-message-format.md` — #25/#26 merged conformant (gating correct); byte-exact stderr WORDING is a tracked non-POSIX-mandated follow-up + how to handle it in the POSIX conformance suites.
- `scope-jobcontrol-fc-behaviors.md` — feasibility scoping of the remaining POSIX-mode job-control (#23–27,#49) + fc (#54–57) behaviors: TRACTABLE vs VERIFY vs CEILING, with the next two-issue fleet round.
- `plan-dynvar.md`, `plan-error-format-pass.md`, `plan-punted-builtins.md` — scoped sub-plans for specific clusters of fixture failures.
- `json-output.md` — bashy's opt-in `set --json` / `declare --json` structured-output extensions.
- `plan-bashy-release-t0.md` — **`bashy release`**: the distribution verb (what bytes leave this machine, under what name — the one thing no orchestration verb owns). T0 = the local-first half in-process over `coreutils/pkg/release`: `bashy release --snapshot` builds → archives → checksums a `.goreleaser.yaml` subset and emits a `bashy-release-v1` ledger, with no network, no credentials and no tag. The whole GoReleaser CLI is NOT imported (measured: +77.3 MB, 277 new modules, 6 new MPL-2.0 deps); the tail (sign/sbom/publish/packages) stays binmgr-managed externals and is refused **by name** when a config declares it, never silently skipped. Records why the atlas group is `toolchains`, and why a snapshot's version is stated (`--version`) rather than guessed from a tag.
- `agent-bands-and-nicknames.md` — the shipped **band** (L1–L4 capability peg, normalized across providers — a vendor's own tier ladder is never mapped positionally) + **nickname** system on `bashy agent`/`bashy model`. Bands live on the model and are inherited by the agent; `--min-band N` selects a roster (`bashy meet start --min-band 3` seats its own table and reports who it skipped). Canonical model names are version-explicit (`opus5`) and the family name (`opus`) is a *derived* alias that re-points itself on release — so a record never rots. Nicknames are assigned deterministically from the binding (same agent, same name, every host). Rules: speak the alias, record the address; a binding is canonicalized however it was spelled; a derived name never shadows a declared one. Read before any fleet-registry / agent-selection / routing work.
- **`bashy craft` + `bashy define`** — the living skill graph and the
  what-is-this-word resolver, both in `coreutils/pkg/{craft,lexicon}`. `craft` is
  the layer OVER the skills catalog: `find` asks for a capability in plain words
  (matching on what a skill GUARANTEES, so a query resolves a skill whose prose
  never used those words), `compose` renders it on demand at a **band** (0 = a
  runnable script needing no model, 4 = pure intent), and `learn`/`facts`/`fold`
  accumulate what running things taught. `define` answers "what is this word on
  THIS system" across verbs, agent bindings, skills, env vars, local commands,
  path segments, interfaces and mounts — reporting a command's resolved path and
  an alias's expansion, and classifying a credential WITHOUT echoing it.
  bashy contributes `internal/agentos/learn.go`, the ExecHandler middleware that
  records what each successful invocation taught. Two rules that bite: `define`
  must never gain a subcommand (its argument is an arbitrary user token, so a
  subcommand steals that word — `bashy define study` once made "what is the word
  study" unaskable), and `lexicon emit` must never render a `Location` (paths
  carry the operator's home dir, and emit writes into committed files). Both
  ratcheted. Design of record: `../docs/skill-graph-design.md`.
- `command-atlas.md` — the Command Atlas: the multi-axis agent-facing catalog of the whole command surface (classical group + execution tier + capability + idiom axes). Tables live in `coreutils/pkg/atlas` (coverage-test-ratcheted against `tool.Names()`); the bashy merge layer is `internal/agentos/atlas.go`; views via `bashy commands --view tier|group|capabilities|effects|web|origin|posix|external|portable`, `--os <goos|any>` (default: this host) and `--portable` filters, `--tier/--group/--cap` filters, `--idioms`, `--atlas` (`bashy-atlas-v1`). Adding a verb/tool = add its atlas entry (the tests name what you forgot). **Since Sprint 167 (1.0.0) the default listing is core-first** (35 core in seven rows + visible extras + userland counts) and every record carries an `origin` (bash · gnu · unix · external · bashy, exclusive) plus a `posix` tag — the bashy-added group is the **yoke commands** (`commands` minus yoke = classic; yoke = built for agentic tools, and agentic ≠ needs a model — deterministic rungs count); 22 yoke commands are **curated-hidden as `experimental`** (`curatedHiddenVerbs` in `agentos.go` — shims and dispatch untouched, `--all` lists them, a gate graduates them). `sandbox`/`peer` are the taught names of the hidden `podman`/`sphere`. See `command-atlas.md` §2.6. **Since Sprint 179 (2026-09-14) the operator can add commands of their own:** `bashy commands add|set|rm|edit|verify|show|list|schema` write a `kind: command` record (exec argv · pinned download · inline script; `BASHY_COMMANDS_DIR`/`_PATH`, no embedded ring) whose atlas entry is DERIVED (`atlas.RegisteredEntry`, origin `registered`) and which resolves through the innermost ExecHandler rung on both `wireExec` branches — POSIX and agentic alike, `VSC_PROFILE=cert` excepted — at the front door, and under `agentic` with the native rules; `internal/agentos/registered.go` is the index, the collision filter (a name bashy ships is refused by holder), and the rung. A CRUD word counts only when a NAME follows it; `command` is the hidden no-shim alias. See `command-atlas.md` §2.9 and the umbrella's `docs/bashy-commands-registry.md`.
- `space-time-advisor.md` — the shipped space-time advisor: non-intrusive error-time hints (cwd/network/compute/disk + doomed-loop + network-fingerprint host memory) that steer agentic tools off doomed retries. Self-contained feature doc (dimensions, env vars, `bashy-advice-v1` JSON schema, scope/non-goals).
- `one-agent-control.md` — **the one control surface** every command that drives an agent CLI now steers through (`invoke` · `weave` · `meet` · `foreman`). `chat.Session` (Start/Say/WaitIdle/Turn) is the primitive — *Invoke is a question, Session is a conversation* — and it lives in `chat` because that is where `agentChildEnv` (secret scrub · single granted API key · shell-forcing · principal identity) lives. `agentpty` owns the wire (`TextFrame` = a sentence typed; `VerbatimFrame` = a keystroke), collapsing three divergent copies of one protocol. Why `meet --steerable` is a flag and not a default (a live turn under a THIRD-PARTY CLI has no boundary — it ends on silence, so it pays a quiet period out and a TUI startup in). **A tool that declares `events_arg:` escapes that**: it reports `turn.end` and bashy believes it, because that is a fact the agent asserted rather than a silence bashy interpreted — today only `ycode` does (see `first-party-harness.md`). Also: `foreman interrupt` (ESC as a real keystroke) — a queued message never reaches an agent stuck in a tool loop, because it reads its queue only between turns and that turn is never going to end. Read before any steering / `say` / `tell` / agent-launch work.
- `activity-events.md` — **the shared activity-event contract**: one compact envelope
  (`bashy-activity-v1`), one interest-routing matrix, one delivery path onto bashy#10's
  EXISTING durable notification and wake primitives — no second mailbox. The envelope
  CANNOT carry a body (there is no body field; 96-byte caps; control characters and
  credential prefixes refused; closed action/status vocabularies), and the event id is a
  time-free hash because a clock in a key degrades at-least-once to once-per-retry.
  Routing is by named relationship only, reported with the delivery, in a fixed
  precedence (mention > assignment > ownership > subscription > dependency > membership);
  reads are never broad-broadcast; the actor is never a recipient. Delivery is
  journal → publish → wake, in that order, so a failed wake costs latency and never the
  event; coalescing and rate limiting DEMOTE, NEVER DROP. Package
  `internal/agentos/activity`, verb `bashy activity`, adapter API consumed by bashy#11.
  Read before wiring any subsystem to notify anybody.
- `unified-inbox.md` — **`bashy inbox` is the one receive-side view** over MB, Meet boards, Bus notifications, and stable role addresses. It adds no store, preserves per-source cursors, watches all sources, and injects one budgeted block only at verified Bashy-owned turn boundaries; externally-started sessions remain explicit pull-only.
- `chat-interactive-launcher.md` — **`bashy chat` as the governed front door** for launching a third-party agent CLI *interactively*: the tool's NATIVE UX (agentpty's raw-mode local-TTY passthrough, not a bashy REPL) but with the fleet-selected model, full `agentChildEnv` governance, and a live-sessions registry (`~/.bashy/sessions/`) that makes the launched agent ADDRESSABLE — `chat sessions`/`steer`/`interrupt`/`attach`, later coach/meet. Selection: `--agent NICK` (specific) or `--band N`/`--tool T` (any operable one, reusing `SeatByBand`). `invoke` stays the one-shot (*Invoke is a question, Session is a conversation* — finally implemented). ycode is special-cased (already bashy-native → just launches it with the resolved `--model`). Companion to `one-agent-control.md`. Read before any interactive-launch / session-registry / chat-mode work.
- `unified-agent-assignment-visibility.md` — **`bashy agent` is the canonical live-work view.** Every managed launch, including short one-shot invoke work, publishes room membership while live; the roster reconciles room, weave, and sprint state without duplicates and exposes named/ad-hoc attribution to humans and JSON consumers. Read before changing any agent launch or assignment surface.
- `absence-of-evidence.md` — **the day's real product, and the codebase's characteristic failure.** SEVEN instances in one day of ONE shape: *a success state reached by the absence of evidence.* Declared fields nothing writes (`ConversationMessage.Usage`, `ExemptFromMasking`, `StreamOptions`, `SessionTotalCost`, 3 config fields), caps that bind and exit 0, a pricing fallback that bills an unknown model at Claude's rate. Every one produced a PLAUSIBLE ANSWER THAT WAS NOT TRUE, and four of them nearly got recorded as facts about a MODEL. Also: the four times my own instruments lied (`cmd | head && echo OK` chains off head's exit; `rm` on a receiver's open file; a bad `pgrep` pattern; an OTLP receiver silently dropping span events). Read before trusting any green check.
- `agentic-history-and-space-graph.md` — **the shipped agentic replacement for the `history` builtin, and the entity graph learned from it.** Two planes from one observation at the ExecHandler seam: TIME (`pkg/execlog`, every dispatched command, ordered, prunable) and SPACE (`pkg/spacegraph`, hosts/endpoints/accounts and the relations between them, bi-temporal, `0600`, **no export path — every node is identity**). `graph learn` pipes what the corpus supports into kb as **candidate** pages carrying an ADDRESS into the stream, not a copy; `graph evidence` walks it back, and reports honestly when the records have been pruned (the claim outlives its evidence). Load-bearing rules: time is never in a key (put a clock in one and the store silently fills with n=1 singletons); FAILURE TEACHES NOTHING (a transport failure is unattributed — correction is by supersession on positive evidence); every read verb prints its coverage. **Read `../docs/knowledge-substrate-reconciliation.md` first** — it demotes these two from "stores" to a stream and a view, with kb as the one truth.
- `observability.md` — the shipped OTel plane. bashy could RUN a collector (`bashy otel`) and fed it NOTHING — it was the one tier of the whole stack missing from the umbrella's `service.name` set. Two primitives, chosen from what six hours of debugging could not see: **Provenance** (a value next to WHERE IT CAME FROM — the only bug caught by a signal was caught by `from_provider=false`) and **BoundHit** (a limit records when it BINDS — especially when the run recovers). Plus a span per command at the ExecHandler chokepoint, including the EXIT CODE. Stack trimmed 286 MB → 109 MB (−61%) by going Victoria-only: jaeger (2,240 deps) → VictoriaTraces, perses (1,478) → vmui, collector (833) → three proxy map entries, prometheus (556) → VictoriaMetrics. Pure standard OTEL env vars; unset endpoint is a total no-op; `cmd/bash` links none of it.
- (umbrella) `docs/bashy-action-model.md` — the action contract: skill, agent, command and script are one `run(input) → output | error` shape; tool is the executor, model a resource; the `kind: skill` record rule and the generic `--set`/`schema` CRUD contract behind `bashy skill|tool|model|agent`.
- `audit-log.md` — the shipped compliance audit trail: a tamper-evident, hash-chained, secret-redacted record of every dispatched command with agent attribution and Command-Atlas effects (`bashy-audit-v1`; NIST AU-3/AU-9). Opt-in via `BASHY_AUDIT`, off by default, never in `cmd/bash` / `--posix`. Read side is `bashy inspect audit {status,tail,verify,export,path}`; core is `coreutils/pkg/policy/audit`, the ExecHandler middleware is `internal/agentos/audit.go`. Records; does not block (policy engine) or contain (OS sandbox) — the un-bypassable record of the agentic+interactive command path, composes with auditd/EDR. Deferred: OTel export, signed checkpoints, gitleaks-grade redactor.
- `fips-140.md` — the shipped FIPS 140-3 build mode: `make build-fips` (`GOFIPS140=v1.0.0`) builds both binaries against the Go Cryptographic Module (CMVP #5247); pure-Go, no cgo/BoringCrypto. Use `GODEBUG=fips140=on` (the build-fips default — keeps `md5sum` working), NOT `fips140=only` (rejects MD5) for a general shell. State surfaced in `bashy inspect doctor` and `bashy inspect context --json` (`runtime.fips140`). A FIPS-built `bin/bash` still passes 86/86. Pairs with the audit log for the FedRAMP/CMMC procurement story.
- `plan-bashy-ask-human-input.md` — **`bashy ask`**: get an ad-hoc value from the
  HUMAN from inside an agent session, over a channel the agent does not own
  (controlling terminal → GUI askpass → out-of-band rendezvous), returning a PATH
  rather than the value. Exists because a command run by an agentic CLI does not
  own its stdin or stdout — measured: Claude Code `setsid`s its children, so
  `/dev/tty` is ENXIO and the obvious implementation cannot work. Replaces the
  `/tmp/x` habit. Engines are `coreutils/pkg/{ctty,ask}`; bashy contributes the
  four registration points. Design of record: `../docs/bashy-ask-human-input-design.md`.
- `bash.md`, `agentic-extensions.md` — background references, not active plans.

POSIX-conformance frontier (the active layer now that bash-5.3 is 86/86 — driven via `suites.md` + `dag.md`):

- `plan-posix-conformance.md` — plan of record for the POSIX-mode conformance push (the differential suites + yash scoreboard).
- `conformance-statement.md` — the standing conformance claim; `shell-conformance-comparison.md` / `cross-shell-conformance-baseline.md` — bashy vs other shells.
- `posix-mode-behaviors.md` — catalogued `--posix` behaviors; `builtin-vs-external-conformance.md` — builtin/external divergence notes.
- `posix-cert-handoff-runbook.md`, `posix-cert-preflight-status.md`, `fidelity-ceiling-assessment.md` — VSC-PCTS certification runbook + status + the hard-ceiling assessment.
- `yash-conformance-gap.md` — the yash-scoreboard failure analysis behind the headline number in `docs/TODO.md`.
- `zsh-scoreboard.md` — the zsh Tier-0 own-suite baseline (`make test-zsh`, `tools/ztst` runner); INFO metric, not a gate.
- `chunked-fleet-conformance-plan.md` — the chunked/fleet/container conformance lanes in `dag.md` (`test-bash-chunks*`, `yash-chunks*`): chunk count is a corpus property pinned in a committed manifest, and the authoritative run stays single-host + unchunked (`make test-bash-container` runs all 86 serially in one hermetic image) — campaign mode never speaks for it.
- `isolated-test-lanes.md` — the multi-agent container/runbook: weave workspace → lane → independent OCI container/result namespace; public self/Bash53/Yash commands, private POSIX A/B/C/D lanes, repeated-profile concurrency, swappable base-image adapters, capacity, status, and cleanup.
- `ci-failure-autorepair-plan.md` + `config/ci-failure-fixer.env` + `scripts/ci-failure-{router,fixer,gate}.sh` — the `.github/workflows/ci-failure-report.yml` lane that routes a CI failure to a **fixer** run (the band-selected agent that repairs one failing gate — a lighter role than the SDLC `conductor`, which is the escalation target for a fix that needs orchestration).
- `bashy-v1.0.0-readiness.md` — the release-readiness ledger.
- `agent-adoption/matrix.md` — which agentic CLIs are verified running on bashy as their shell (the `force-agent-shell` skill's evidence base).
- `first-party-harness.md` — **why ycode is in the fleet, and what it actually buys.** All four "still owed" items shipped 2026-07-14. The differentiator is NOT that it wins a bake-off (it lost — slowest, most code): it is the **event channel**. `--events` emits `turn.start`/`tool.call`/`turn.end` as NDJSON on both the one-shot and TUI paths, so a turn's end is a FACT THE AGENT REPORTS rather than a silence bashy interprets (`WaitIdle`, 25s). `turn.end.text` equals `--print` stdout exactly. Not yet reached: server mode (the agent loop lives in the server process, which never sees the client's `--events`). Read before any harness-selection or `chat.Session` work.
- `plan-agent-harness-positioning.md` — gap analysis and phased plan for positioning
  bashy as a **harness kit**, not a privileged Go-coded harness: governed substrate,
  conductor, authoring kit, and bidirectional peer. Records the plain-bash worked-loop
  gate, the later Bash++ port, and why front-door verbs need the same P0 governance seam
  as shell-resolved commands.
- `plan-bashy-llm.md` — P0.5 design of record for the missing stateless model-call
  primitive: one JSON request/response, model resolution through the fleet catalog,
  Ollama + OpenAI-compatible T0 providers, no tool execution, and record/replay. Read
  with the positioning plan before any native model-call or harness-authoring work.
- `band-ladder.md` — **the L1–L4 ladder across every provider**, with the two open questions now ANSWERED by running both as conductors: `gemini3.1` demoted L3→L2 (9.4× repeat ratio, never converged — a coder, not a lead; confound recorded), `deepseek-v4-pro` CONFIRMED L3 (1.2×, decomposed and delegated unprompted). The loop metric — total tool calls ÷ distinct — is the cheapest conductor health check there is. Read before any band re-peg or conductor selection.
- `fleet-live-verification.md` — `bashy agent verify --live`: why a STRUCTURAL check (both halves of a binding resolve in the catalog) is not evidence that an agent can speak, and how five dead bindings hid behind one that looked healthy. The origin of "a verifier that passes on the ABSENCE of a known failure is not a verifier."
- `harness-ab-deepseek.md` — **the three-harness A/B** (ycode vs opencode vs aider, one model, one task, one gate). All three converge; the differences were in the HARNESS, and two were ours. Headline finding: **all three exit 0 when they fail** — a harness's exit code carries no information, so run the gate. Also why aider is retired from the API-key lane (it cannot discover the files a task needs — architecture, not quality) and why opencode is KEPT (the cross-check against a first-party bug). Read before any harness-selection or fleet-routing decision.

Per-fixture cluster analyses + blocker ledgers (snapshots — diff line-counts and PASS/FAIL claims in them are dated, re-measure before trusting):

- `ARITH-ANALYSIS.md`, `ARRAY-ANALYSIS.md`, `ASSOC-ANALYSIS.md`, `DBG-SUPPORT-ANALYSIS.md`, `NAMEREF-ANALYSIS.md`, `NEWEXP-ANALYSIS.md` — failure-cluster breakdowns for the named fixtures.
- `NEWEXP-RESIDUE-R2.md`, `ERRORS-ANALYSIS-R2.md` — round-2 residue analyses.
- `ERRORS-BLOCKERS.md`, `HEREDOC-BLOCKERS.md`, `HISTORY-BLOCKERS.md`, `QUOTEARRAY-BLOCKERS.md`, `VARENV-BLOCKERS.md` — per-fixture blocker ledgers.

Weave-round verification + retro reports (historical, not load-bearing):

- `QA-REPORT-R10.md`, `JUDGE-REPORT-R6.md`, `JUDGE-REPORT-R7.md`, `SPRINT-R10-RETRO-DRAFT.md`.
