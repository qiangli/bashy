# The compliance audit log

`bashy` can record a **tamper-evident, hash-chained log of every command the
shell dispatches**, with agent attribution and secrets stripped. It is the
evidence half of the security uplift — the artifact a security team needs to
answer the one question they cannot answer today about an agent-driven machine:
*"what did the agents run here, and prove it."*

Opt-in, **off by default**. Never linked into the lean `cmd/bash` drop-in, never
active under `--posix`.

## Why bashy is the place to do this

`auditd` is root-only, Linux-only, and cannot see a shell's own dispatch; every
agentic CLI's self-reported log is written by the thing being audited. bashy is
the execution point an agent drives, so it is the one place the record exists
and cannot be bypassed *by the agent* — there is no un-audited exec path through
the shell.

**Honest scope.** This records; it does not block (that is the policy engine's
job, a later phase) and it cannot see across an `execve` into the process a
command spawns (that is the OS sandbox's job). It is not a total chokepoint:
`python -c …`, `make`, or any compiled binary can spawn children without asking
bashy. It is the complete, un-bypassable record of the **agentic + interactive
command path** — the new, previously unmonitored surface — and it composes with
`auditd`/EDR for the rest. It also does not yet cover pure shell builtins
(`export`, `cd`): those are handled by the interpreter before the exec handler.
Every *external command and coreutils tool* is recorded.

## Turning it on

```sh
export BASHY_AUDIT=1                 # log to the default path
export BASHY_AUDIT=/var/log/bashy.jsonl   # or an explicit path
```

Default path: `$BASHY_HOME/audit/audit.jsonl`, else `~/.bashy/audit/audit.jsonl`.
The file is created `0600` in a `0700` directory (NIST AU-9 — the audit trail
must not be readable or writable by other users).

This is distinct from the sh engine's low-level `BASHY_AUDIT_LOG` raw-event hook
(one `AuditEvent` per simple command, no outcome). Two layers, two env vars:
`BASHY_AUDIT` is the enriched, chained compliance record.

## The record

One JSON object per line (`bashy-audit-v1`), mapping to NIST SP 800-53 **AU-3**
(who / what / when / where / outcome) and close to the OCSF `process_activity`
class:

```json
{"schema":"bashy-audit-v1","seq":2,"prev_hash":"sha256:…","time":"2026-…Z",
 "actor":{"human":"dhnt:agent/007","agent":"claude","model":"opus-4.8","uid":501,"pid":2077},
 "action":"exec","argv":["rm","-f","/tmp/x"],"binary":"rm","cwd":"/repo",
 "effects":["destroy"],"host":"…","decision":"allow","exit":0,"duration_ms":3,
 "hash":"sha256:…"}
```

- **`actor`** — the agentic novelty over `auditd`: attribution to a tool + model
  + session (from `BASHY_PRINCIPAL` / `BASHY_AGENT_*` / `BASHY_SESSION` the fleet
  launcher injects), not just a uid.
- **`effects`** — the Command Atlas security classification (`bashy commands
  --view effects`), so each record is self-describing about what the command
  could do: `rm` is `destroy`, `git` is `cred,net,…`, `dag` is `remote`.
- **`argv`** — post-expansion and **redacted**: `--token X`, `-p X`, and
  `SECRET=…` values are masked (`‹redacted›`). This is the minimal built-in pass
  so the log is not itself a secret-leak; a gitleaks-grade streaming redactor is
  the next design item.
- **`decision`** — `allow` only for now (there is no enforcement yet); it becomes
  meaningful when the policy engine ships.

## Tamper-evidence

Records form a hash chain: `hash = H(prev_hash ‖ this-record-without-its-hash)`,
rooted at a public genesis. Deleting, reordering, or editing a single byte of
any record breaks every record after it.

```sh
bashy audit status    # on/off, path, record count, chain state
bashy audit tail [N]   # the last N records (JSON lines)
bashy audit verify     # walk the chain, report the first break (exit 1 if broken)
bashy audit export     # the full chain + a verification summary (evidence bundle)
bashy audit path       # print the configured log path
```

`verify` is the load-bearing one: `chain intact: N records verified ✓`, or
`CHAIN BROKEN at seq K … (this record was altered)`.

Concurrent bashy processes sharing one log are safe: every append takes an
exclusive file lock (`flock`), re-reads the head under it, and links — so two
processes cannot fork the chain or tear a line. On a platform without `flock`
(Windows), prefer a per-process log path.

## Compliance mapping

One artifact satisfies several frameworks at once: NIST 800-53
**AU-2/3/9/10/12**, SOC 2 **CC7**, ISO/IEC 42001 clause 9, and EU AI Act
**Art. 12** (record-keeping). The plan is to also emit each record as an OTel log
over the umbrella telemetry plane so it lands in an existing SIEM with no mapping
— that, plus signed chain checkpoints and the gitleaks-grade redactor, is the
next slice.

## Code

- `coreutils/pkg/policy/audit/` — the reusable core: `Record`, `Actor`, the hash
  chain, locked append, `Verify`, `Redact`. stdlib-only; the caller supplies the
  classification and identity.
- `bashy/internal/agentos/audit.go` — the ExecHandler middleware (outermost in
  `WireExec`) that enriches with atlas effects + actor and captures the outcome.
- `bashy/internal/agentos/audit_verb.go` — the `bashy audit` read verb.
