# v1 command release evidence

`release-bar-v1.json` is the explicit v1.0 command inventory and its release
metadata. `scripts/release-bar.py` joins it with the candidate binary's
`commands --json --all` and `commands --atlas --json --all`. The generated
[table](release-bar-v1.generated.md) includes the named gaps; the matching
JSON is machine-readable. Execution tier is separate from stability.

Generate and enforce the bar with:

```sh
go build -o /tmp/bashy-release-bar ./cmd/bashy
python3 scripts/release-bar.py --bashy /tmp/bashy-release-bar \
  --candidate "$(git rev-parse HEAD)" \
  --json-out /tmp/release-bar.json --markdown-out /tmp/release-bar.md --check
```

`--check` exits 1 for any missing point; report-only mode omits that flag.
Exit 2 means invalid inputs or failed catalog collection. Named gaps are
outstanding work, never exclusions or a known-failure baseline. The committed
snapshot identifies a development candidate by binary SHA-256 and carries no
three-OS pass claim.

The **v1 command release bar** workflow (`release-bar.yml`, manual dispatch)
builds the selected commit and runs `TestE2EAllListedCommandsDispatch` natively
on Linux, macOS and Windows. Each runner uploads its report, raw `go test
-json` log and pass record. The final job requires exactly one report from
each OS, the same candidate and inventory, matching successful JSON probe
versions, and positive dispatch test plus package completion. It uploads the
combined table even when the strict check fails. Missing artifacts, a skipped
test, a failed package, or a pass from another candidate cannot satisfy it.
Normal CI also runs the focused generator regressions.

The schema column records the version observed **inside** a successful JSON
response to the displayed invocation, not a schema guessed from a capability
flag. Probes use temporary home/config/store directories and a 30-second
bound; only version and failure category are retained. A failed probe is an
evidence gap, not proof that the command has no JSON support. A passing probe
covers that invocation, not every subcommand or payload field. Array-shaped
legacy outputs cannot gain an envelope without a consumer migration; they
remain named gaps. The additive `skill probe` version preserves existing keys.

Front-door envelopes (Story 1533, part 4d–4h). Where a verb's body lives in
yoke and still writes a bare JSON array, the bashy front door adds the
envelope on the way out rather than changing the probe to an unrelated
subcommand. `tool|model|agent|skill list --json` (and the hidden plurals)
emit `bashy-registry-list-v1` (`kind`, `view`, `items`), written by the
registry itself since Sprint 406; the front door wraps only a list verb
that still writes a bare array and passes an envelope through untouched. The
other probed surfaces in that slice are bashy-owned: `conform --list --json`
(`bashy-conform-v1`), `install-agent --json` (`bashy-install-agent-v1`),
`mcp tools --json` (`bashy-mcp-tools-v1`, the profile `serve` would expose,
without serving), `out --list --json` / `out --json HANDLE` (`bashy-out-v1`),
`ollama status --json` (`bashy-ollama-status-v1`, read-only — never
provisions or contacts the engine), and `chat --dry-run --json`
(`bashy-chat-v1`, resolves the launch without running an agent). `llm env
--json` emits the `bashy-llm-env-v1` envelope.

Consumers name shipped skills with a source reference; empty cells require a
real consumer to be identified. Visible/core does not imply a stability tier.
The tier is declared as `atlas.stability` in yoke's `pkg/atlas`, or in bashy's
`bashyOwnedVerbAtlas` for bashy-owned rows. A policy JSON `stability` override
takes precedence per row and records its `stability_source`; no promotion is
inferred.

The inventory preserves plural aliases and canonical front doors. It includes
readiness checks, the reference agent's human front door, and the audit verb.
Language syntax (`set`/`declare` extensions, decorators), modifiers (`--dry-run`,
`--web-ui`), and explicitly deferred surfaces are outside this verb table.
`supervise` remains a named scope conflict. `foreman` is still listed by the
binary but deliberately suppressed from the shared atlas (Bashy #40); resolving
that release contradiction requires an owner decision. `sshd` remains an
implementation gap until its owning story lands.

To record an existing run, pass its unmodified `go test -json` log to
`--record-dispatch LOG --candidate SHA --os OS --run-url URL`. Feed all three
resulting records to generation using repeated `--dispatch-evidence FILE`.
The candidate label must identify the code actually built and tested; do not
reuse an older run or relabel a dirty build as a release commit.
