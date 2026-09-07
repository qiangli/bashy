# An agentic action as a function, script and tool

Sprint 134 uses one bare `agentic` contract. This example takes text and returns
a summary or an error. The implementation explicitly uses the existing governed
one-shot chat launcher. It does not change ordinary commands or enable repairs.

Select an agent already configured on your host, then run from the bashy checkout:

```sh
export SUMMARY_AGENT=your-configured-agent
bashy --bashpp examples/agentic/typed.bpp 'The build passed. Deployment is pending.'
printf '%s\n' 'The build passed. Deployment is pending.' | bashy --bashpp examples/agentic/summarize.bpp
./examples/agentic/summarize.bpp 'The build passed. Deployment is pending.'
```

`actions.bpp` declares a shell function.
`typed.bpp` declares a typed function and receiver method, preserving the typed
output/error pair. `summarize.bpp` calls the shell function and preserves shell
streams and status; the executable file is also a command/tool. Each entry script
opts in through its own block. The flags `--plain --read-only` select plain output
and the launcher's existing read-only policy. No agent or model is installed by
these examples. The provider's existing permissions and budget policy apply.

## Native tool embedding

The runnable Go example uses the standard shell CLI, `agentos.WireExec`, and
coreutils' existing tool registry. It registers `example-summary` only in the
example binary, not as a new standard bashy verb:

```sh
go run ./tools/agentic-example --bashpp -c 'agentic { example-summary "The build passed. Deployment is pending."; }'
printf '%s\n' 'The build passed. Deployment is pending.' | go run ./tools/agentic-example --bashpp -c 'agentic { example-summary; }'
```

`tool.RunContext.Ctx` already retains `interp.HandlerContext.Agentic`. The adapter
checks that bit before reading input or invoking the provider, uses shell argv,
stdin and cwd, and writes output/errors through the shell's streams. The production
runner is nil, selecting `chat.Invoke`'s governed launcher. Tests inject a
deterministic runner into that same adapter and exercise real parser/interpreter,
`WireExec`, registry dispatch and chat invocation; they do not call a model.

`SUMMARY_AGENT` comes from the invoking shell's environment. Chat's fleet and
credential configuration remain host configuration, as with other embedded chat
callers. The adapter never changes process environment or cwd. Existing explicit
`bashy chat` calls retain external dispatch: intercepting them in-process would
change shell-local environment semantics because that CLI reads process globals.
`BASHY_AGENTIC` retains its existing meaning and supplies no language permission.

Implementation plan: reuse the already-preserved handler context, keep the adapter
inside this embedding example, verify success/error/cancellation and ordinary
dispatch, and use the existing CLI for the shipped script presentations. Compiled
Bash++ packaging and parity remain Sprint 117 work.
