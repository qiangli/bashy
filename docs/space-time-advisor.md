# Space-time advisor

A **non-intrusive supervisor** built into the `bashy` AgentOS shell (never the
pure `bash` drop-in). When a command **fails**, it appends one short, advisory
line explaining that the failure is determined by the agent's ambient resource
environment — and what to do instead — so an agentic tool (codex, claude, aider,
opencode, …) stops the *doomed retry loop* it would otherwise burn time and
tokens on.

It is the same idea as a senior engineer glancing over your shoulder and saying
"that host isn't on this network — stop probing it." Only `bashy` is positioned
to say this: it holds the whole-environment snapshot, the accumulated history of
what worked under which conditions, **and** sees every command in the retry loop.

## Design invariants

- **Error-time only.** It runs after a command exits non-zero. It never fires on
  success and never inspects a command before running it (that is dry-run's job).
- **Never blocks.** It is a post-exec `ExecHandler` middleware that returns the
  underlying exit status unchanged and only writes one extra line. Worst case it
  adds a wrong hint — advisory, ignorable.
- **Opt-in / off by default for humans.** Active in agent mode (`BASHY_AGENTIC`) or
  when `BASHY_ADVISOR` is set on-ish. Silent in an ordinary interactive session.
- **Never in the drop-in.** It lives in `internal/agentos` (imported only by
  `cmd/bashy`) and is inert under `--posix`, so `bin/bash` never links or runs it.

## The two axes

### Space — the ambient resource environment

A command fails *for a reason rooted in where it runs*. The advisor checks these
dimensions (each conservative, to keep false positives low) and reports the first
that explains the failure:

| Dimension   | Fires when…                                                      | Example hint |
|-------------|------------------------------------------------------------------|--------------|
| **cwd**     | a relative path arg is missing here but present at the repo root | "`foo.go` does not exist in the current directory but is present at the repo root (`/repo`) — you are likely in the wrong directory." |
| **network** | a network tool fails to reach a LAN-ish host that does not resolve from here | "`host.local` looks like a LAN-only address and does not resolve from here — you may be off its network; use the tunnel/public route." |
| **compute** | exit 137 (SIGKILL) on a memory-heavy build/test tool             | "exit 137 means the process was killed (SIGKILL), typically the OOM killer … reduce parallelism (-j) or batch it." |
| **disk**    | the filesystem backing `$PWD` is read-only or nearly full        | "the volume backing the current directory has only 1.0 MB free … the failure may be ENOSPC." |

Platform coverage: cwd and network work everywhere. The **disk** dimension needs
`statfs` (unix only). The **compute** RAM figure is read from `/proc/meminfo`
(Linux); elsewhere the OOM hint still fires on exit 137, just without the byte
count.

### Time — accumulated memory

The advisor remembers across commands and sessions, which turns a guess into a
grounded statement:

- **Doomed-loop detection (per session).** The *identical* command failing three
  or more times in a row escalates the hint to `retryable=false` with "this has
  failed repeatedly for the same reason — change the approach, not the
  parameters." Catches an agent re-running the same thing expecting a different
  result.
- **Host-success ledger (persisted).** Successful connections to a host are
  recorded under a **network fingerprint** — a hash of the machine's current
  physical-network subnets (loopback, link-local, down, and virtual/container/VPN
  interfaces are excluded so it tracks the real network, not transient overlays).
  When that host later fails to resolve under a *different* fingerprint, the hint
  upgrades to the concrete: "`host.local` was reachable before from a different
  network but does not resolve here — this machine has moved off its network."
  The fingerprint is a *relative* signal ("same network as before?"), so no
  unreliable absolute "home vs remote" classification is attempted.

## Output

In **agent mode** the advice is one JSON object per failure, on **stderr** (so
stdout — the command's real output — stays clean):

```json
{"schema_version":"bashy-advice-v1","kind":"advice","dimension":"network",
 "command":"ssh","exit":255,"retryable":false,
 "hint":"\"host.local\" … this machine has moved off its network.",
 "suggest":"reach it via the tunnel or its public/VPN address; …"}
```

In **human mode** (`BASHY_ADVISOR=1` interactively) it is a single prose line:

```
bashy: ⓘ "host.local" … this machine has moved off its network. reach it via the tunnel …
```

Schema is versioned (`bashy-advice-v1`); fields: `schema_version`, `kind`
(`"advice"`), `dimension` (`cwd`|`network`|`compute`|`disk`|`loop`), `command`,
`exit`, `retryable`, `hint`, `suggest`.

## Configuration

| Variable | Effect |
|----------|--------|
| `BASHY_AGENTIC` | agent mode: advisor on, JSON output (set by weave / agent harnesses) |
| `BASHY_ADVISOR` | explicit control: `0`/`false`/`off`/`no` force-disable (even in agent mode); `1`/`true`/`on`/`yes` force-enable; unset ⇒ on in agent mode, off for humans |
| `BASHY_ADVISOR_NOMEM` | `1`/`true`/`yes`/`on` disables the persisted host ledger (the in-memory session layer still works) |
| `BASHY_ADVISOR_STATE` | override the ledger path (default `<user-cache>/bashy/advisor/hosts.json`) |

The ledger is local to the user's cache directory, written atomically and merged
across concurrent `bashy` processes, capped to the most-recently-successful 256
hosts. All persistence is best-effort: any IO error is swallowed and never
surfaces to the shell.

## Scope / non-goals

- It is **advisory**, not an enforcer — it does not block, sandbox, or alter
  commands. (A hard-block tier was explicitly left out.)
- It explains failures from environment dimensions it can *observe locally*. It
  does **not** model remote-side state it cannot see (e.g. a remote host's uid).
- It depends on **no other bashy feature** — in particular it does not require
  the (separate, deferred) execution audit journal / result-object work. It is
  complete and self-contained as shipped.

## Source

- `internal/agentos/advisor.go` — middleware, space probes, pattern library, emission.
- `internal/agentos/advisor_memory.go` — failure counter, host ledger, network fingerprint.
- `internal/agentos/advisor_{unix,other}.go` — `statfs` disk probe (unix) / stub.
- `internal/agentos/advisor_ram_{linux,other}.go` — `/proc/meminfo` RAM probe (Linux) / stub.
- Wired as the outermost `ExecHandler` middleware in `agentos.WireExec` (non-posix branch).
