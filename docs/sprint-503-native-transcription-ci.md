# S503 native transcription workflow admission

`.github/workflows/s503-native-transcription.yml` is a separate six-runner
workflow triggered when the reviewed admission record is pushed to
`s503-contract-integration`. It is deliberately
**blocked** until the operator evidence and exact native dependencies can be
admitted. A build or a downloaded media file cannot turn the job green.

The first job requires
`docs/sprint-503/evidence/native-transcription-admission.json`. That reviewed
record does not yet exist. Its schema is
`s503-native-transcription-admission-v1` and it must include the six runner
labels as keys under `deno`, each with `archive_url`, `archive_sha256`, and
`executable_sha256`, and under `whisper`, each with a reviewed native
`executable_sha256`. It also requires the admitted yt-dlp v2026.10.21 zipapp,
ggml-tiny.bin hash and byte count, both reviewed speech excerpts, their review
evidence, the controlled WebM hash and duration, and the operator media hash and
duration. The existing Windows amd64 and Darwin arm64 Deno pins cover only two
of six labels. Six native whisper executable digests and the operator excerpt
are not recorded in the repository.

After admission, each native job verifies OS, Python and Go architecture,
builds candidate Bashy with `scripts/go-product.sh`, builds the seed helper and
pure-Go WebM/Opus decoder, and verifies hashes on yt-dlp, model, Deno archive,
Deno executable and the pinned whisper.cpp source build. It uploads partial
identity and dependency evidence on failure. The explicit terminal failure in
the current workflow marks the next missing implementation: prepare the two
operator URL media files by invoking `source_youtube.py` on the canonical and
short URL, require both to match the admitted SHA-256, create the exact private
manifest for `verify-youtube-pipeline.py`, and run its `--online` path. That
harness checks script and skill orientations, TXT/SRT/VTT excerpts and timing,
receipts, failure cleanup and cancellation. A second run with `--offline` must
be launched under **external, proved network denial**. The current harness
only records an enforcement string; it cannot enforce network isolation.

There is no reviewed Windows network-denial wrapper for the hosted runner yet.
Linux/macOS containment also needs a native runner proof. Until those steps
are implemented and the six actual result artifacts are inspected, S503.8
remains **0/6 full native transcription passes**. This workflow intentionally
cannot produce a successful matrix verdict in its current state.
