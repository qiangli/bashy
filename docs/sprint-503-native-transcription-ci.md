# S503 native transcription workflow admission

`.github/workflows/s503-native-transcription.yml` is a separate six-runner
workflow triggered when the reviewed admission record is pushed to
`s503-contract-integration`. It is deliberately
**blocked** until the operator evidence and exact native dependencies can be
admitted. A build or a downloaded media file cannot turn the job green.

The first job requires
`docs/sprint-503/evidence/native-transcription-admission.json`. That reviewed
record does not yet exist. Its schema is
`s503-native-transcription-admission-v2` and it must include the pinned whisper
build run `40195543786` and source commit
`c59c3aa134588e21e1039750515fc00159cc2cb6`. It must include the six runner
labels as keys under `deno`, each with `archive_url`, `archive_sha256`, and
`executable_sha256`, and under `whisper`, each with a reviewed native
`executable_sha256`. It also requires the admitted yt-dlp v2026.10.21 zipapp,
ggml-tiny.bin hash and byte count, both reviewed speech excerpts, their review
evidence, the controlled WebM hash and duration, and an `operator_media` object
with separate `canonical` and `shortened` SHA-256 and review evidence. The
shortened hash must be measured independently; the two URLs may produce equal
bytes. Each of the six `offline_canary` entries requires a reviewed OS policy
method, enforcer SHA-256 and external probe command. The canary report itself
must be generated on that runner, with the policy still active during inference.
The existing Windows amd64 and Darwin arm64 Deno pins cover only two of six
labels. The operator excerpt and six external offline policy implementations
are not recorded in the repository.

After admission, each native job verifies OS, Python and Go architecture,
builds candidate Bashy with `scripts/go-product.sh`, builds the seed helper and
pure-Go WebM/Opus decoder. It downloads the exact platform artifact from run
`40195543786` with the official `actions/download-artifact` action and
`GITHUB_TOKEN` `actions: read` permission. The runner verifies its binary
against the pinned per-target SHA-256, build report, source archive SHA-256,
identity JSON and SHA-256 manifest, then verifies the GitHub attestation against
the build workflow, source commit and integration ref. It copies the verified
binary to the coordination cache. This deliberately avoids rebuilding a macOS
binary whose SHA-256 can change between native builds. The job also verifies
hashes on yt-dlp, model, Deno archive and Deno executable. It uploads partial
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
The Windows candidate is temporary per-program outbound block rules for
Python, Bashy, Deno and each adapter/whisper executable, with an external
Python network canary and `try/finally` rule cleanup. A global outbound block
can disconnect the hosted runner itself. macOS `sandbox-exec` `deny network*`
passed on a separate claimed host, but still needs proof on the GitHub runner.
Linux `unshare -n` likewise needs a hosted-runner capability check and canary;
failure to enter the namespace must stop the job. The canary report must name
the running policy and show a failed real connection attempt before and during
the offline replay. Until those steps
are implemented and the six actual result artifacts are inspected, S503.8
remains **0/6 full native transcription passes**. This workflow intentionally
cannot produce a successful matrix verdict in its current state.
