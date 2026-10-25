# Local transcription seed adapters

These are ordinary registered command implementations, outside core dispatch.
They consume the shared `bashy-transcribe-stage-v1` JSON contract; the command
and the editable Bash# unit use the same records as user-provided adapters.
This directory supplies no pipeline, catalog, receipt implementation, tool
installer or exception to dependency admission.

Build on an authorized development/build host, from the bashy module:

```sh
go build -o /absolute/path/transcribe-adapter ./examples/transcribe/adapters
bashy commands add transcribe-local --set exec.0=/absolute/path/transcribe-adapter \
  --set exec.1=local --set effects.0=read --set effects.1=exec
```

Registration uses `bashy commands add`, as the acquisition example above shows.
Use the command names in the table as suggested local IDs, each with the same
helper path and its corresponding operation (`local`, `wav`, `ffmpeg`, `whisper`,
`export`, or `model`) as a fixed argument. Decode/infer/export additionally need
`write` and `destroy` effects; model setup also needs `net`. Inspect/edit records
through ordinary `commands show` / `commands set` / `src` workflows. This
checkpoint does not install seed records automatically: final applicability
and dependency declarations await the registry integration story.

| Command | Stage | Behavior |
|---|---|---|
| `transcribe-local` | acquire | Read a regular local file, return its absolute path and SHA-256; preserve the original. |
| `transcribe-wav` | decode | Stream RIFF PCM16, mix 1–8 channels to mono and linearly resample to 16 kHz; no external decoder. |
| `transcribe-ffmpeg` | decode | Extract first audio stream and normalize using an explicitly configured, reviewed decoder. No automatic discovery or download. |
| `transcribe-whisper` | infer | Verify cached model, run whisper.cpp with `--no-gpu` and language `auto` by default, parse and validate timestamped JSON. |
| `transcribe-export` | export | Write TXT, SRT or VTT; publish atomically without overwriting an existing destination. |
| `transcribe-model-setup` | setup | Explicitly download and checksum-verify a named model through `binmgr.EnsureWhisperModel`. |

The PCM seed supports ordinary little-endian PCM16 WAV at 1–192 kHz. It uses
linear interpolation, not a mastering-quality resampler. Other WAV encodings,
compressed audio and video need an independently selected decoder. RIFF sizes,
chunk boundaries, audio frames and the normalized result are validated. Empty
audio and a video with no audio stream fail. The FFmpeg adapter restricts input
protocols to local files and never accepts tool arguments from stage data.

Model setup is separate from inference:

```sh
bashy transcribe-model-setup base
# After the pipeline unit is installed:
bashy transcribe --model base --language auto --format srt \
  --source transcribe-local --decoder transcribe-wav \
  --provider transcribe-whisper --exporter transcribe-export \
  --output 'speech output.srt' 'speech input.wav'
```

`default` means multilingual `base`. Model IDs, sizes and SHA-256 digests come
from the existing binmgr catalog. Inference only reads that verified cache; it
does not download missing weights or repair corrupt entries. English-only
models require explicit `--language en`; use multilingual models for automatic
language detection. BCP 47 tags select their primary language (e.g. `es-MX`
becomes `es`). Successful inference requires detected language, nonempty text,
ordered segments and timestamps within the decoded audio duration. CLI exit
status alone is insufficient: whisper.cpp can exit zero without a transcript.

The managed whisper.cpp manifest must contain a signed, committed artifact pin.
Until admitted artifacts are available it fails with `missing_tool`. An
independently reviewed deployment can configure an exact executable with
fixed `--whisper PATH --whisper-sha256 DIGEST` arguments before the `whisper`
operation; the helper checks those executable bytes before each invocation.
Configure the `ffmpeg` operation with fixed `--ffmpeg PATH --ffmpeg-sha256 DIGEST`
arguments **only after decoder policy admission**.
A digest proves byte identity, not license admission or build provenance; record
both separately. Nothing here bundles, downloads or admits FFmpeg.

Each helper invocation is `transcribe-adapter [deployment flags] ADAPTER
REQUEST.json`. Deployment flags precede the adapter name. Paths are argv
elements, including paths containing spaces or shell metacharacters. Invoke a
stage through `bashy transcribe stage STAGE --provider COMMAND REQUEST.json`.
Direct invocation and registered invocation use identical JSON and validation.

Decode and infer require an existing `work_dir`. The pipeline owns that
directory and removes it on every exit. Each adapter creates a private child;
failed decode removes it, successful decode transfers its lifetime to the
caller, and inference always removes its own JSON scratch. Original input and
cached model files are never removed by these stages. Independent stage callers
must clean their `work_dir`. Export writes a sibling temporary file, closes and
hashes it, then publishes with an exclusive hard link; errors preserve existing
output and remove the temporary file. The output filesystem must support hard
links. This avoids a check-then-overwrite race on all supported local filesystems.

Subprocesses receive context cancellation. Cancellation yields a nonzero result
with `error.code=cancelled`; failures contain no success artifacts or segments.
The helper reserves stdout for one JSON result, and bounds captured stderr.

## Verification

```sh
go test -v ./internal/transcribeseed ./internal/transcribecontract ./examples/transcribe/adapters
go test ./internal/agentos -run TestTranscribe -count=1
go vet ./internal/transcribeseed ./internal/transcribecontract ./examples/transcribe/adapters
```

The subprocess fixtures test implementation behavior only. Synthetic WAV data,
fixture transcript text and a test executable do not establish native inference,
language recognition or production decoder acceptance.

For real native verification, create a private JSON manifest with `os`, `arch`,
`model`, `config` and `fixtures`. `config` contains `whisper_path`,
`whisper_sha256`, `ffmpeg_path`, `ffmpeg_sha256`. Each fixture has `path`, `kind`
(`audio` or `video`), `decoder` (`wav` or `ffmpeg`), expected `language`, and a
reviewed reference excerpt `contains`. Supply both audio and video and at least
two languages. Paths must be valid on the verified native host. Run after model
setup, with the host claimed and its actual architecture verified:

```sh
BASHY_TRANSCRIBE_NATIVE_MANIFEST=/private/native-fixtures.json \
  go test -v ./internal/transcribeseed -run '^TestNativeTranscription$' -count=1
```

This gate checks actual content, auto-detected language, all exports, timing
bounds and original-input preservation, and logs media/output hashes and
inference duration. It blocks Go HTTP requests after setup. That is not OS-level
network isolation: repeat under externally enforced network disablement and
record the enforcement method. Also run the same media through the editable
pipeline's skill and script orientations; this package-level gate does not
prove those integration criteria. Runtime evidence, policy admission and
six-platform coverage remain separate from a passing focused test suite.

CLI schema reference: the pinned upstream
[whisper.cpp v1.9.5 CLI](https://github.com/ggml-org/whisper.cpp/blob/v1.9.5/examples/cli/cli.cpp)
uses JSON `result.language`, `transcription[].offsets` in milliseconds and
`--language auto` for detection during transcription. `--detect-language`
exits after detection and is intentionally not used here.
