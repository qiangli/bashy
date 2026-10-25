# Local transcription adapter implementation and verification

Sprint 503 / S503.4 uses the shared v1 stage contract in
`internal/transcribecontract`; the original agentos types are aliases. The only
additive wire field is optional `work_dir`, owned by the caller. It is required
by seed decode/infer stages so intermediates have an explicit cleanup owner.

The implementation plan and delivered structure are:

1. Keep orchestration and adapter selection outside this implementation. Build
   a standalone helper whose entry points are ordinary registered commands.
2. Read local media without mutating the source. Stream normalized PCM WAV in
   Go; isolate external decoder support behind a reviewed executable config.
3. Reuse binmgr's model catalog/setup. Infer from verified cached models only,
   explicitly on CPU, with strict JSON/content/timestamp validation.
4. Render TXT/SRT/VTT and atomically publish only completed exports. Preserve
   existing destinations and remove only files created by the current stage.
5. Exercise failures and cancellation using focused fixtures, then verify
   actual transcription separately on authorized native hosts with admitted
   dependencies and reviewed audio/video references.

The seed implementation is not imported by shell dispatch and adds no startup
initialization. It creates no alternative registry, receipt, command runtime or
tool provisioner. User adapters can replace any stage without changing it.

See [the adapter guide](../examples/transcribe/adapters/README.md) for setup,
individual stages, cleanup ownership, limitations, and the real native fixture
manifest/gate. Pure PCM normalization uses linear interpolation; a separately
selected media decoder handles compressed formats and video.

Focused verification on the initial candidate passed all 11 implementation test
groups, the existing `TestTranscribe` tests and the shared-contract compatibility
test. Seed/contract/helper `go vet` passed. A separate registration smoke used an
isolated ordinary command ring and verified acquisition through a helper path
and media path containing spaces/metacharacters. No production transcript was
used for these tests.

`TestNativeTranscription` is opt-in and was skipped in that development gate.
S503.4 acceptance remains open pending real admitted whisper.cpp binaries,
decoder policy, actual multilingual audio/video inference and all three export
formats under an enforced network-off repeat, plus integration with the
selection and dual-orientation pipeline stories. A green fixture suite is not
native transcription evidence.

The registration continuation adds `examples/transcribe/adapters/register.sh`
using the integrated generic command registry, with role/request applicability,
platform declarations and effects. It keeps seed records in the ordinary local
ring, with explicit model setup and optional independently admitted executable
configuration. The focused registration smoke exercises actual local acquisition,
PCM normalization, TXT/SRT/VTT export and pipeline publication/cleanup; its
inference substitution is labelled fixture data. This advances implementation
integration only. Real whisper.cpp inference, video decoding policy, multilingual
speech and externally enforced offline evidence remain acceptance prerequisites.

Continuation verification: all five `test_registration.py` groups pass, including
matching TXT/SRT/VTT bytes and valid receipts in the command/skill orientations.
The candidate selector was built at `9290f1c431dc`; the helper uses Bashy
`fe77d5bf840c` with Yoke `b449cd4b0926`. The local changes are registration,
documentation and this smoke test only. Module-mode seed tests and focused vet
also pass. These are development fixture results, not native speech evidence.
