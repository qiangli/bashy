# S503 WebM/Opus adapter implementation plan and outcome

Story #2740, Story-ID `44bebdcd7762`.

1. Preserve the pinned feasibility module. Its asynchronous convenience reader
   cannot safely carry track errors, cancellation and DiscardPadding.
2. Add a standalone ordinary decode adapter module under
   `examples/transcribe/webm-opus`, using exact MIT Pion v0.1.0 and an authored
   synchronous EBML parser. Retain codec/timing/track metadata, support all
   lacing forms and skip video; reject unsupported/ambiguous mappings.
3. Bound bytes, leaves, nesting, elements, tracks, packets and decoded samples.
   Apply trim/gain, normalize to 16 kHz mono PCM16, atomically publish in a
   unique owned work child, and return v1 stage JSON. Preserve input/unrelated
   files on every result. Check cancellation without background demux workers.
4. Register with provides/accepts/effects through the existing command registry;
   do not edit core dispatch, source adapters or the pipeline.
5. Gate with malformed/cancel/trim/video/lacing tests, real non-silent CC0 speech
   and authored stereo against independent reference PCM, bounded fuzz/race/vet,
   exact module license closure and an isolated real-registry smoke.

All five implementation/development steps are delivered. Detailed supported
subset, bounds, reproducible commands, fixture attribution and measured
results: [adapter README](../examples/transcribe/webm-opus/README.md).
Native operator-media transcription and six-target release acceptance remain
separate gates; controlled unit evidence does not close those story criteria.
