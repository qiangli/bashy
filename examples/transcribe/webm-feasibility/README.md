# WebM/Opus feasibility checkpoint

Sprint 503, Story 2740, Story-ID `44bebdcd7762`. Development/unit evidence only.
No decoder is registered or admitted by this directory. It is a separate module
so evaluating these libraries does not add dependencies to Bashy or Yoke.

The bounded slice takes the authorized feasibility fallback. Pion's codec API
is usable, but the evaluated WebM convenience reader cannot safely implement
the requested invalid-media, cancellation and precise timing contracts without
additional demuxing work. This is not a finding that a pure-Go adapter is
impossible. No FFmpeg dependency, runtime download or external codec is used.

Run the characterization gate from this directory:

```sh
GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout=20s ./...
GOWORK=off go mod verify
```

The six tests assert observed capabilities and limitations, including an
expected crash isolated in a child **unit-test process**. Passing this gate
confirms those findings; it does not mean a decoder or transcription passed.
The controlled container is generated in memory from authored structs and a
three-byte CELT silence packet. No upstream audio fixture is copied. This is
not speech fidelity or inference evidence.

Observed gate on 2026-10-25: all six named tests PASS with `CGO_ENABLED=0`
(package time 0.244 s); all six also PASS with `-race` (1.296 s).
`go mod verify` reports all modules verified. The first trim-fixture failure
and its correction are recorded below. No native adapter, VM, container, OCI
or inference runtime was executed. The undeclared-track test's expected child
failure is part of the passing characterization, not an application success.

## Exact pins and license closure

| Source | Exact version / commit | Reviewed license and scope |
|---|---|---|
| [pion/opus](https://github.com/pion/opus/tree/eb60bfd0e51a58e122fcfd0c0df027b014db7daa) | `v0.1.0`, `eb60bfd0e51a58e122fcfd0c0df027b014db7daa` | MIT (`LICENSE`, `LICENSES/MIT.txt`, Go SPDX headers). `.reuse/dep5` marks upstream seed/fuzz data CC0-1.0; that data is not used here. |
| [at-wat/ebml-go](https://github.com/at-wat/ebml-go/tree/e3fb7f90b76d91d81f073b8bc95a567c2962332a) | `v0.19.4`, `e3fb7f90b76d91d81f073b8bc95a567c2962332a` | **Apache-2.0, not BSD-3-Clause** (`LICENSE`, source headers). No required external modules in its go.mod. |
| [ebml-go/ebml-go](https://github.com/ebml-go/ebml-go/tree/a6981b91c79ce851e0fd9974f2894e2b9daae65f) | `v0.0.0-20121201001202-a6981b91c79c`, `a6981b91c79ce851e0fd9974f2894e2b9daae65f` | BSD-3-Clause at repository root. Rejected alternative: legacy dependencies below have no established pinned license closure in this slice. |

The first two source archives and module manifests are pinned by `go.sum`.
The rejected legacy archive is `h1:ZeImyLGxznWLIjq0RW3bikw+QvKWhAeizJc5GQBfjks=`;
its synthetic module manifest is `h1:yE2sPZCK5hInFuqjYM3n4ZZZPmDDgDuyllmQRn7x1ho=`.
It is not in the executable probe's dependency graph.

`GOWORK=off go list -deps -test` shows only the standard library, the probe,
Pion Opus and at-wat EBML. Pion's declared module graph additionally includes
the following upstream test dependencies, all pinned in this probe's go.sum:

| Module | Version | License text inspected |
|---|---|---|
| github.com/stretchr/testify | v1.11.1 | MIT, LICENSE |
| github.com/davecgh/go-spew | v1.1.1 | ISC, LICENSE |
| github.com/pmezard/go-difflib | v1.0.0 | BSD-3-Clause, LICENSE |
| gopkg.in/yaml.v3 | v3.0.1 | MIT for listed libyaml-derived files; Apache-2.0 for remaining files, LICENSE and NOTICE |

These four modules are not linked by the probe. Pion and at-wat's runtime
package closure uses only their own packages and the Go standard library
(BSD-3-Clause Go distribution). Retain applicable license/copyright notices
and the Apache obligations in any future shipped artifact. This source review
is not a release SBOM, vulnerability audit, signature or six-target admission.

The legacy `ebml-go/ebml-go/webm` package cannot be treated as a standalone
permissive demuxer merely because its root license is BSD. `adecoder.go`
unconditionally imports `code.google.com/p/ffvorbis-go/ffvorbis`, and
`vdecoder.go` imports `code.google.com/p/ffvp8-go/ffvp8`; it also imports
`code.google.com/p/ebml-go/ebml` and `github.com/petar/GoLLRB/llrb`. There is no
upstream go.mod pinning this closure, and its audio decoder is Vorbis, not
Opus. The licenses/versions of those legacy dependencies remain unreviewed;
no permissive or pure-Go claim is made for that closure and none is installed.

## API findings

1. **Codec feasible:** `opus.NewDecoderWithOutput(16000, 1)` followed by
   `DecodeToInt16(packet, pcm)` returns samples per channel. A 1,920-element
   buffer covers 120 ms at 16 kHz mono. The controlled 20 ms packet yields
   exactly 320 zero-valued samples. Empty input and an undersized output buffer
   return errors. This does not prove arbitrary speech decoding fidelity.

2. **Malformed track can terminate the process:**
   `mkvcore.NewSimpleBlockReader` dispatches with `r := br[b.TrackNumber]`
   and then `r.f`/`r.closed`, without checking that `r` exists
   (`mkvcore/blockreader.go`, dispatcher goroutine). A container declaring
   track 1 but carrying track 2 triggers a nil-pointer panic. The test proves
   this even with `WithOnFatalHandler` configured. Recovering around the
   constructor or around `Read()` cannot catch a different goroutine's panic.

3. **End trim disappears:** the convenience reader's local `blockGroup` only
   includes Block and ReferencePriority. Its public `Read()` returns packet,
   keyframe and timestamp; no DiscardPadding. The probe decodes the full EBML
   first to establish 0 versus 10,000,000 ns padding, then shows identical
   convenience-reader results. Decoding both packets to PCM would yield 320
   samples in each case, whereas the padded packet needs 160 end samples
   removed at 16 kHz. The public `webm.BlockGroup` also omits DiscardPadding.
   A custom struct with `ebml.Unmarshal` can retain it, so this is specifically
   a convenience-API blocker.

4. **Closing a track does not stop or join the parser:** `Close()` closes only
   its per-track channel. The parser goroutine continues reading the supplied
   `io.Reader`; the probe holds that reader at a gate and confirms Close
   returns without releasing it. The caller must own and cancel the source,
   drain/close all tracks and await parser completion. Ignoring a video track
   without closing/draining it can block dispatch to the selected audio track.
   There is no context or parser Wait method in this API.

5. **Errors need a separate completion channel:** on truncated input, track
   `Read()` returns EOF while `WithOnFatalHandler` receives unexpected EOF.
   The default fatal callback panics. EOF alone must never publish a WAV or a
   successful stage result. The probe demonstrates this separation.

6. **Leaf sizes can be bounded:** v0.19.4 provides
   `ebml.WithMaxLeafElementSize`, passed through `mkvcore.WithUnmarshalOptions`.
   The probe rejects an oversized leaf before allocating its payload. Do not
   incorrectly claim the pinned release has no size-limit API. This option
   alone does not enforce total bytes, nesting, tracks, duration or frame count.

Initial probe result: five tests passed and the trim test failed because
minimal-width Marshal encoded the positive 10,000,000 ns signed value such
that Unmarshal returned -6,777,216. The controlled fixture was corrected to
explicit `DiscardPadding,size=8`; the full-parse assertion now establishes the
intended value. This fixture correction does not fix or bypass the reader's
metadata loss. No upstream encoder is assumed in the production proposal.

## Work required before registering the adapter

Use a synchronous bounded demuxer (or a reviewed upstream/fork fix), retaining
TimecodeScale, CodecDelay, OpusHead pre-skip/gain/channel mapping and
BlockGroup.DiscardPadding. Validate track references before dispatch; select
one supported Opus audio track, skip video safely, and reject unsupported
mapping/encryption/ambiguous audio. Bound leaf size, depth, total input and
output, and packet counts. Handle lacing, sample-accurate trim and nonzero or
discontinuous timestamps explicitly. Do not subtract CodecDelay and pre-skip
twice. Validate cancellation and parser termination before publishing output.

The existing stage contract is `bashy-transcribe-stage-v1`, stage `decode`,
one regular input artifact and required `work_dir`. A future adapter should
reuse `workspace`, `wavHeader` and `decoded` in `internal/transcribeseed`:
create a unique owned child directory and exclusive mode-0600 `audio.wav`,
write little-endian PCM16 at 16,000 Hz / one channel, finalize the RIFF lengths,
close and validate before computing SHA-256. Return only the owned WAV,
`audio/wav`, SHA-256 and media duration/rate/channels; preserve source identity.
Every error/cancellation removes the owned child and returns no artifacts or
segments. Cancellation uses the existing `cancelled` code and status 130;
success transfers cleanup to the caller. Input and unrelated work_dir files
must survive every failure.

Registration belongs in ordinary command records (`provides=transcribe.decode`,
version and WebM applicability, read/write/destroy/exec effects), with explicit
provider selection and ambiguity tested through the existing selector. Keep
the orchestrator, YouTube source adapter and Yoke dependency management free of
decoder branches. Do not register a decoder that only passes this silence probe.

Remaining tests include representative stereo/speech, pre-skip, gain, trim,
lacing, video plus audio, timestamps, malicious sizes/depth/track references,
late truncation, pre/mid-decode cancellation with parser termination, owned
cleanup on every failure, and exact output/stage contracts and registration.
Native operator video, offline/multilingual inference and all six native
OS/architecture targets remain open under the existing story acceptance.
