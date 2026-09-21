# Write your own fence — the rod

Every built-in fence row is a *fish*. This directory is the *rod*: adding a
language or a toolchain bashy has never heard of takes a **runner** and
nothing else. Two ways to provide one:

```sh
# (a) register it once, name it from any script
bashy commands register awk-runner --set exec.0=$PWD/examples/runners/awk-runner.sh --set effects.0=exec
~~~awk as aw !awk-runner
{ words += NF }
~~~
n := aw.run("file.txt")
```

```sh
# (b) inline, in the script itself — a Bash# func or a shell function
func builder(verb string, file string, args ...string) string { ... }
~~~zig as z !builder
~~~
bin := z.build()
```

A runner is called as `runner <verb> <file> [args…]` in the caller's
directory with the caller's environment. Its one reserved verb, `methods`,
answers the verb table — one JSON object per line: `name`, optional
`signature`, optional `effects` — and that answer *is* the alias: nothing is
guessed, an empty answer is a refusal. Every other verb serves the fence: the
materialized body is `$2`, stdout is the value, a non-zero status is the call
error. A verb's declared `effects` are what `@guard` checks, exactly as for a
built-in row. The runner resolves to a function in the script, a builtin, or a
command bashy dispatches itself (`bashy commands register`) — never to a
program on PATH, so a fence means the same thing on every host.

| example | shape | runner | provisioning |
|---|---|---|---|
| `awk.bsh` | interpreter | `awk-runner.sh`, registered | bashy's own awk |
| `zig.bsh` | compiler | inline `func builder` | `"$BASH" zig` — the toolchain bashy already provisions for its C fences |
| `env.bsh` | config | inline shell function `env-runner` | none |

`make smoke-runners` registers `awk-runner` in a private ring and runs the
three on the installed binary. What a built-in row can do that a runner
cannot today — shadow entries, the go overlay, being another fence's module,
lowering — is listed in `bashsharp/docs/fenced-text-blocks-plan.md`
§Rod vs fish.
