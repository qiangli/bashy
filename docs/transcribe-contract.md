# Transcription command and stage contract (v1)

`bashy transcribe [options] <file-or-url>` accepts exactly one input. It
delegates to a registered Bash# unit named `transcribe-pipeline`. The front door
passes the file path or URL as one argument, never as shell source. Register
the pipeline with `bashy commands add`; its implementation and unit receipt
use the existing Bash# skill/script and `unit.Attestation` paths. Until the
pipeline unit is registered, the command reports an unavailable provider.

Options: `--model ID` (default `default`), `--language TAG` (default `auto`),
`--output PATH`, `--format txt|srt|vtt` (default `txt`), `--provider NAME`
(inference provider), `--source NAME`, `--decoder NAME`, and `--exporter NAME`.
The front door passes `--device cpu`; the pipeline may choose a verified local
model for `default`. Empty provider options leave selection to the pipeline.
The `--output` default and local model ID must be resolved by that unit, not by
the front door. Explicit selections take precedence over automatic selection.
The seed implementation and automatic applicability rules are S503.2–S503.4.

## Stage invocation

Each stage is independently callable:

```text
bashy transcribe stage acquire --provider SOURCE request.json
bashy transcribe stage decode  --provider DECODER request.json
bashy transcribe stage infer   --provider INFERENCE request.json
bashy transcribe stage export  --provider EXPORTER request.json
```

`SOURCE`, `DECODER`, `INFERENCE`, and `EXPORTER` are ordinary registered
commands. `stage` invokes the named command with the request path as one
argument and passes through stdout, stderr and exit status. It does not install
a new plugin loader or process runner. A missing provider is an error. The
stage name is recorded inside the JSON request and result; the pipeline must
check it against the stage it invoked. An adapter may be replaced by changing
only the selected registered command; the other stages and their JSON are
unchanged. S503.2 owns automatic selection and applicability declarations.

## JSON data contract

Both request and result use `"version":"bashy-transcribe-stage-v1"` and a
`stage` of `acquire`, `decode`, `infer`, or `export`. This is stage data, not a
second unit envelope or receipt. The Go representations and validation are in
`internal/agentos/transcribe_contract.go`. Unknown optional JSON fields may be
ignored by v1 consumers; a different version must be rejected.

```json
{
  "version": "bashy-transcribe-stage-v1",
  "stage": "infer",
  "artifacts": [{"path": "/tmp/audio.wav", "media_type": "audio/wav"}],
  "media": {"duration_ms": 1200, "sample_rate_hz": 16000, "channels": 1},
  "model": "default",
  "language": "auto",
  "work_dir": "/tmp/transcribe-run-123"
}
```

An `acquire` request has `input` set to the original file or URL. Its result
has at least one artifact path. `decode` receives an artifact and returns a
normalized audio artifact plus media metadata. `infer` receives that artifact
and returns nonempty `segments` with integer `start_ms`, `end_ms`, and `text`.
`work_dir` is an optional absolute per-run directory owned by the pipeline;
acquire, decode and infer adapters may place intermediate artifacts there.
The pipeline removes it on exit. The export `output` path is the caller's
resolved destination and is not removed with `work_dir`.
`export` receives segments, `format`, and optionally `output`; it returns the
created transcript artifact. Successful results include `version`, `stage`,
and the required artifact or segments. Failed results contain
`"error":{"code":"...","message":"...","retryable":false}` and no
success artifacts or segments; the command also exits nonzero. Segment times
are nonnegative, ordered, and have `end_ms > start_ms`. Artifact paths are
local file paths; the unit owns cleanup of its temporary artifacts and must
not report a partial output as success.

Media may carry `duration_ms`, `sample_rate_hz`, `channels`, `language`, and
`source`. Artifacts may carry `media_type` and `sha256`. These fields survive
stage handoff when known. More metadata may be added without changing v1
required fields. An inference provider may report detected language in media.

V1 handles one finite input. Live streams, playlists, diarization, and visual
OCR are outside this contract.

## Existing seams and outstanding prerequisites

This inventory preceded the command design. `bashy commands add` and
`internal/agentos/registered.go` supply registered exec/script dispatch,
argument binding, effect declarations and local override. `src_registered.go`
maps a registered command back to editable source. `yoke/pkg/fleet/commands.go`
owns command declarations; `yoke/pkg/binmgr/command.go` supplies managed
external invocation. `yoke/pkg/unit` provides the shared attestation type,
used by dag and skills; stage data must not masquerade as that receipt.
Sprint 420's frontmatter, contract/receipt and identity stories and Sprint
458's source and Bash# unit MVP are recorded as delivered on their cards.

Unfinished Sprint 503 prerequisites are S503.2 (adapter registration,
applicability and overrides), S503.3 (the one Bash# pipeline unit and both
orientations), S503.4 (local acquisition, decode, inference and export
adapters), and S503.6 (admitted model/tool artifacts). The command does not
claim transcription succeeds before those stories pass. S503.5 adds YouTube
acquisition later. Sprint 499 voice interaction is separate work and is not
required for batch transcription.

## Registering and selecting adapters

A transcription adapter is an ordinary registered command. The command record
uses the existing ring and atlas fields and adds generic provider metadata:

- `provides` contains open dotted roles. The pipeline asks for
  `transcribe.acquire`, `transcribe.decode`, `transcribe.infer`, or
  `transcribe.export`; these are values, not a core enum.
- `accepts` contains zero or more RFC 6901 JSON-pointer/RE2 pairs. All pairs
  must match the stage request. An empty list accepts every input for the
  declared role. Website hosts, filename suffixes, MIME strings and export
  formats therefore remain adapter data.
- `requires` is the same applicability expression used by skill
  `metadata.requires`. Use `has=COMMAND` for executable dependencies and
  `os=` / `arch=` for the supported coordinate. The command's existing `os`
  field is also enforced.
- `effects` is the existing command effect declaration. It is not inferred
  from the adapter role or input.

For example, a source for one independently defined fixture host can be
imported with the ordinary command mechanism:

```yaml
name: fixture-source
kind: command
exec: [/absolute/path/fixture-source]
provides: [transcribe.acquire]
accepts:
  - path: /input
    pattern: '^https://media\.example\.test/'
requires: 'os=linux,darwin,windows arch=amd64,arm64 has=curl'
effects: [read, write, exec, net]
```

```text
bashy commands add fixture-source.yaml
bashy commands show fixture-source --yaml
bashy commands list --json
bashy transcribe select acquire acquire-request.json
bashy transcribe select acquire --provider fixture-source acquire-request.json
```

The selector prints exactly one registered command name. No match reports the
providers rejected by input, dependency or platform declarations; multiple
matches report every ambiguous name and the explicit stage option that resolves
it. `--source`, `--decoder`, `--provider`, and `--exporter` remain authoritative:
the pipeline validates the named command's role, host requirements and input
declaration through `select --provider`, then invokes that registered command.

Shared command directories remain read-only and the normal local ring remains
the override. `bashy commands set fixture-source ...` materializes and edits a
local copy; neither selection nor dispatch has a separate override store. A new
adapter is thus registration data plus its executable body: it requires no
pipeline edit, dispatch case, website table, media-type enum, or Bashy rebuild.
