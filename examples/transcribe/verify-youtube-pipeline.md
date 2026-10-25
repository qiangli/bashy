# Native YouTube pipeline gate

`verify-youtube-pipeline.py` runs the unchanged registered source/decode/infer/export
pipeline with real native binaries and cached model weights. It creates a private
command and skill ring for each run. It does not install dependencies, run an
inference substitute, or make a passing claim from acquisition alone.

Copy `youtube-pipeline-manifest.example.json` to a private path and fill every
path, checksum, candidate commit, decoder provenance and human reviewed excerpt.
The retained operator media must be the independently rehashed WebM from the
canonical and shortened acquisitions. The manifest pins their known SHA-256 and
links run165 Windows amd64 and run168 Darwin arm64 evidence. The controlled
WebM must contain actual speech and be a separate file. Its reference excerpt
and duration must be reviewed against that media. A successful decoder record
requires a separately admitted permissive implementation; a hash alone is not
license admission. Do not set decoder `exec` to FFmpeg for this gate.

On a claimed native host, after model and binary setup:

```sh
python3 examples/transcribe/verify-youtube-pipeline.py /private/manifest.json --validate-only
python3 examples/transcribe/verify-youtube-pipeline.py /private/manifest.json \
  --output-dir /private/evidence/pipeline-online-media
```

The default gate reuses retained operator media through the registered local
source, so it does not download it again. It verifies both the controlled and
operator WebM through script and skill orientations, TXT/SRT/VTT text and
ordered bounded subtitle timing, fuzzy reviewed excerpt thresholds, published
file hashes, command/skill receipt identity, failure and cancellation cleanup.
The normal output directory retains transcripts and `result.json` privately.
Use a new output directory for every run; existing evidence is never replaced.

For a live URL composition proof, explicitly add `--online`. That invokes the
registered YouTube source once with the canonical URL in script orientation and
once with the shortened URL in skill orientation; it downloads media twice.
The retained local runs cover all three formats without further downloads.
The source adapter's focused tests cover unavailable URLs and download child
cancellation. This gate also checks one invalid URL in online mode and local
pipeline cancellation in every mode.

After setup, disable network access outside this process, record the enforcement
method in `offline_enforcement`, and rerun with `--offline` into a new output
directory. Offline mode uses only retained local media and does not call the
YouTube source. The harness records the method but cannot independently prove a
host firewall or disconnection; the native evidence must include that proof.

The gate refuses missing, changed, or unreviewed inputs before invoking the
pipeline. `--validate-only` checks the manifest and media without inference.
`python3 -m unittest examples/transcribe/test_verify_youtube_pipeline.py` runs
the focused pure validation tests on a development host. They are not native
transcription evidence.
