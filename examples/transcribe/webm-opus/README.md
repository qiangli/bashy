# WebM/Opus decode adapter

Sprint #503, Story #2740, Story-ID `44bebdcd7762`.

A standalone, pure-Go **implemented** `bashy-transcribe-stage-v1` decoder.
It uses the reviewed MIT Pion Opus **v0.1.0** with a synchronous bounded EBML
parser authored here. It never calls at-wat's convenience reader, starts a
parser goroutine, invokes FFmpeg, downloads a runtime, or changes the pipeline.
The previous `../webm-feasibility` remains a separate characterization module.

## Build, register, use

Build on an authorized development/build host; the adapter has its own module:

```sh
cd examples/transcribe/webm-opus
GOWORK=off CGO_ENABLED=0 go build -trimpath -o /absolute/path/transcribe-webm-opus .
BASHY_BIN=/absolute/path/bashy ./register.sh /absolute/path/transcribe-webm-opus
bashy transcribe select decode request.json
bashy transcribe stage decode --provider transcribe-webm-opus request.json
```

Registration creates an ordinary command record with
`provides=transcribe.decode`, v1/decode/`.webm`/absolute `work_dir` applicability and
`read,write,destroy,exec` effects. The executable receives the request filename
as one argument. Paths are never evaluated as shell code. Other matches cause
the existing selector's ambiguity error; explicit provider selection resolves
it. The extension requires no core, source adapter, pipeline, or Yoke edits.
OS/architecture declarations describe build support, not native acceptance.

```json
{
  "version": "bashy-transcribe-stage-v1",
  "stage": "decode",
  "work_dir": "/absolute/existing/owned-work-dir",
  "artifacts": [{"path": "/absolute/input.webm", "media_type": "audio/webm"}],
  "media": {"source": "/absolute/input.webm"}
}
```

Exactly one regular input is required; symlinks, pipes and devices are refused.
An optional input SHA-256 is verified. The caller owns `work_dir`; the adapter
creates a unique mode-0700 child and mode-0600 `audio.partial`. It writes mono
PCM16 at 16 kHz, finalizes RIFF lengths, syncs, hashes and closes, then atomically
renames to `audio.wav` in that child. Only then does it return the WAV path,
media type, SHA-256 and normalized rate/channel/duration metadata. Source media
identity fields are copied. Duration is whole milliseconds, rounded down.
Successful child cleanup transfers to the caller. Errors, cancellation and a
failed JSON write remove only this invocation's child. Input and unrelated
work files are preserved. Cancellation returns `error.code=cancelled`, status
130 and no artifacts; invalid requests use status 2, other failures status 1.
Publication is atomic within the owned child, not a multi-file transaction.

## Supported subset and timing policy

- One WebM EBML header and Segment; finite or unknown-sized Segment/Clusters.
  Info/Tracks may occur before or after clusters. Finite unknown metadata is
  skipped within parent bounds. Known structural fields in the wrong parent
  are rejected. Header read versions and integer widths are validated.
- Exactly one Opus audio track, optionally accompanied by video tracks.
  Track numbers and UIDs must be nonzero/unique; every block must reference a
  declared track, including video. Video payloads are validated/delaced and
  skipped without decoding or blocking audio. Multiple audio tracks, even if
  one is a preferred/default track, are rejected as ambiguous.
- OpusHead version 1, mapping family 0, mono/stereo, matching channel metadata
  and 48 kHz declared Opus sampling frequency. Header input-rate is advisory.
  OpusHead, track identity, CodecDelay, TimecodeScale and signed DiscardPadding
  are retained in the parsed representation. Gain is applied before PCM16
  saturation. Other mapping families, content encodings/encryption, block
  additions/codec-state changes and track timestamp transforms are rejected.
- CodecDelay must match OpusHead pre-skip to within one nanosecond. It describes
  the same delay and is **not** subtracted a second time. Pre-skip is consumed
  across packets. Signed DiscardPadding trims the whole BlockGroup, including
  laces; negative padding and pre-skip remove the union of their leading ranges.
  Trims round to the nearest 16 kHz sample (62,500 ns). Padding greater than its
  block's decoded duration is rejected.
- SimpleBlock and BlockGroup, no lacing, Xiph, fixed-size and signed-delta EBML
  lacing are supported. Empty/overrunning/disabled laces are rejected.
- The first selected audio timestamp becomes output time zero. Nonzero and
  negative origins are therefore normalized explicitly; leading timeline
  silence is not synthesized. Subsequent block timestamps must agree with the
  accumulated **untrimmed** packet duration within one TimecodeScale tick plus
  one output sample, accommodating container timestamp quantization. Larger
  gaps/overlaps are rejected. This is continuous-audio transcription, not an
  edit-list renderer. Absolute timestamps above 2^50 ns in magnitude are refused.
- Incomplete trailing EBML/Opus fails the whole stage, even after valid audio.
  The codec call has a same-goroutine panic boundary; a decoder panic becomes
  an invalid-media failure and cannot publish partial audio. No `recover` is
  used to declare malformed data successful.

## Hard bounds

| Resource | Limit |
|---|---:|
| Request JSON | 1 MiB |
| Input file | 128 MiB |
| Leaf payload | 8 MiB |
| Parsed master nesting | 16 |
| EBML elements | 100,000 |
| Tracks | 32 |
| Packets, including skipped video | 300,000 |
| Packet submitted to Opus | 65,536 bytes |
| Laced frames per block | 256 |
| Decoded PCM | 256 MiB, before trimming |
| Per-packet decoded audio | 120 ms |

The input is read into bounded memory in 64 KiB chunks. Parsed payloads alias
it; it is not copied per leaf. This is bounded in-memory demuxing, not streaming
arbitrarily large files. Codec/output buffers are limited by packets and laces.
Context is checked at reads, elements, structure validation, packets, blocks,
hashing and publication. No background parser survives a return. Cancellation
cannot interrupt a kernel regular-file syscall already in progress; it is
observed at the next bounded operation boundary.

## Focused development gate

```sh
GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout=20s ./...
GOWORK=off go test -race -count=1 -timeout=20s ./...
GOWORK=off go vet ./...
GOWORK=off go mod verify
GOWORK=off go test -run '^$' -fuzz '^FuzzDemux$' -fuzztime=3s -parallel=1 -timeout=15s
GOWORK=off go test -run '^$' -fuzz '^FuzzOpusPacket$' -fuzztime=3s -parallel=1 -timeout=15s
BASHY_BIN=/existing/development/bashy ADAPTER=/built/transcribe-webm-opus python3 test_registration.py
```

The registry smoke creates an isolated registry/home and verifies real
registration, selection, stage execution, WAV shape/hash, ambiguity and explicit
override. It needs an existing Bashy containing the S503 selector. It does not
build Bashy or perform inference.

Development evidence, 2026-10-25: normal tests and race tests pass; vet and
module verification pass. Three-second single-worker fuzz runs executed
153,046 demux inputs and 24,176 codec inputs without a failure. The 2-second
speech fixture produces 31,896 output samples, 31,756 nonzero, 91.81 dB SNR
against independent reference PCM. The stereo fixture produces 9,496 samples,
9,492 nonzero, 99.54 dB SNR. The test requires non-silence and at least 20 dB
SNR; it does not equate a silence packet with speech correctness. Trim,
metadata/gain, all lacing forms, video coexistence, limits, late invalid input,
invalid mappings, cancellation and preservation have controlled tests.

The isolated real-registry smoke also passed against an existing **older
S503 development binary**, not a rebuilt current release candidate. This is
adapter development evidence only. Native operator WebM, multilingual/offline
inference, script/skill full pipelines and six-platform release acceptance are
later gates coordinated with the native pipeline owner. No VM/container,
runtime install or inference was performed by this gate.

See [THIRD_PARTY_LICENSES](THIRD_PARTY_LICENSES) and
[test fixture provenance](testdata/README.md). A release still needs the normal
SBOM/vulnerability/signature/distribution checks and native target gates.
