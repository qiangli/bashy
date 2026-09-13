# Harness-neutral `bashy kb` stage recipes

These recipes wire the same ordinary commands at lifecycle points owned by
three different harnesses. The harness adapts its event payload into the shell
variables below; it does not gain a private knowledge API.

Use this exact command set in every adapter:

```sh
bashy kb context --for "$PROMPT" --rings repo,host --budget 700
bashy kb note add --candidate --ring agent --episode "$SESSION" --title "$TITLE" --body "$BODY"
```

When a harness also owns a gate, record the verdict and promote only from the
returned event id:

```sh
bashy kb observe --ring agent --episode "$SESSION" --kind gate --ref "$GATE_REF" --ran --passed --command "$GATE_COMMAND" --exit-code 0 --where "$GATE_WHERE" --json
bashy kb validate "$SLUG" --ring agent --from-gate "$EVENT_ID"
```

Omit `--passed` and supply the real nonzero `--exit-code` for a failed gate.
Never manufacture an event for a gate that did not run, and never promote from
a span, transcript, commit message, or manual claim.

## Claude Code hooks

Wire `UserPromptSubmit` to the context command. Map the hook payload's `prompt`
to `PROMPT`; its stdout is the bounded context block supplied to the model.

Wire `Stop` to the candidate-note command. Map the hook session id to `SESSION`,
and let the adapter choose a distilled `TITLE` and `BODY`; do not ingest a raw
transcript. Suppress the note command's ordinary stdout if the hook protocol
requires JSON-only output.

The hook configuration should call small reviewed adapter scripts from the
repository. Keeping payload parsing in those scripts avoids embedding shell
quoting and JSON parsing in the hook configuration itself.

## Codex hooks

Use repository-local `.codex/hooks.json` entries for `UserPromptSubmit` and
`Stop`, pointing to reviewed adapters with the same variable mapping and exact
commands above. Codex sends hook input as JSON on stdin: `prompt` is available
on `UserPromptSubmit`, while `session_id` and `last_assistant_message` are
available at the relevant lifecycle points. Return the context command's plain
stdout from `UserPromptSubmit`; return valid hook JSON from `Stop` after
suppressing the note command's stdout.

Review and trust the repository hooks before relying on them. Codex runs hook
commands in the session working directory, so the ordinary repo ring resolves
without a harness-specific path.

## Plain `bashy chat` agent

A shell wrapper owns the same two seams. Before launch, bind the requested task
to `PROMPT`, run the context command, and pass its stdout with the instruction:

```sh
CONTEXT=$(bashy kb context --for "$PROMPT" --rings repo,host --budget 700)
bashy chat --agent "$AGENT" --instruction "$PROMPT" --context "$CONTEXT"
```

At the wrapper's successful turn boundary, distill any durable lesson and run
the same candidate-note command. If the wrapper runs a gate, use the same
observe/validate pair above.

## YAML-defined agents

Do not duplicate a fourth recipe here. The ycode repository's
`examples/agent.yaml` is the Y2 reference wiring: its `bashy.run` nodes execute
these commands and bind the JSON context envelope to the knowledge port.
