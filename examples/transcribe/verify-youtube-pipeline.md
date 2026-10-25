# Native YouTube pipeline gate

`verify-youtube-pipeline.py` runs the unchanged registered source/decode/infer/export
pipeline with real native binaries and cached model weights. It creates a private
command and skill ring for each run. It does not install dependencies, run an
inference substitute, or make a passing claim from acquisition alone.

Copy `youtube-pipeline-manifest.example.json` to a private path and fill every
path, checksum, candidate commit, decoder provenance and human reviewed excerpt.
`candidate_commit` must be the full HEAD of the Bashy checkout. Supply the
SHA-256 of every `source_sha256` file shown in the example manifest; the gate
checks each against both the working file and the bytes committed at HEAD.
The retained operator media must be independently rehashed from the canonical
and shortened acquisitions. Record a separate `sha256` for each URL; equivalent
video IDs do not require identical downloaded bytes. The manifest links run165
Windows amd64 and run168 Darwin arm64 acquisition evidence. The controlled
WebM must contain actual speech and be a separate file. Its reference excerpt
and duration must be reviewed against that media. A successful decoder record
requires a separately admitted permissive implementation; a hash alone is not
license admission. Do not set decoder `exec` to FFmpeg for this gate.

On a claimed native host, after model and binary setup:

```sh
python3 examples/transcribe/verify-youtube-pipeline.py /private/manifest.json --validate-only
python3 examples/transcribe/verify-youtube-pipeline.py /private/manifest.json \
  --online --output-dir /private/evidence/pipeline-online-media
```

The default gate reuses retained operator media through the registered local
source, so it does not download it again. It verifies both the controlled and
operator WebM through script and skill orientations, TXT/SRT/VTT text and
strict cue structure with ordered bounded timing, fuzzy reviewed excerpt thresholds, published
file hashes, command/skill receipt identity, failure and cancellation cleanup.
The normal output directory retains transcripts and `result.json` privately.
Use a new output directory for every run; existing evidence is never replaced.

For a live URL composition proof, add `--online`. The gate invokes each URL
through both script and skill orientations in TXT format, four downloads in all.
It then exercises the same unmodified decode, infer and export stages with
retained canonical media through both orientations and all three formats.
These two crossed checks prove URL acquisition is independent of orientation
and downstream format selection without 12 downloads per native target. The
shortened retained file is independently hashed and identity checked; the gate
does not transcribe that retained file separately.
The source adapter's focused tests cover unavailable URLs and child
cancellation. In online mode, the native gate starts an additional admitted
yt-dlp download at 32 KiB/s, waits for a nonempty partial file, interrupts the
process group, and requires a structured `cancelled` result with no media or
partial files left. Every mode starts a local pipeline, waits until its infer
request exists, interrupts the process group, and requires a failed result with
no published output or owned temporary files. Windows sends CTRL_BREAK_EVENT;
POSIX sends SIGINT. The native run, rather than unit tests, decides whether
these operations work on each host.

After setup, use a native OS policy to disable network access outside this
process. While that policy is active, run an external network canary and save a
JSON report with `schema: s503-network-denial-v1`, native `os` and `arch`, the
same `candidate_commit`, `network_denied: true`, nonempty `probe_command`,
nonzero `exit_code`, and nonempty `error`. Set `offline_enforcement` to an
object with `method`, absolute `canary_report` path, and `canary_sha256` of that
report. Keep the policy active while rerunning with `--offline` into a new
output directory. A string assertion of offline mode is rejected. The canary
report is external evidence; native OS policy logs and proof that it remained
active through the run must accompany the result. Offline mode uses only
retained local media and does not call the YouTube source.

Offline replay reports download cancellation as `not_tested_offline`, because
network access is disabled. Its online counterpart must report `pass`. A native
host that cannot interrupt its process group or clean the partial download
fails the gate; no status shortcut converts that failure into a pass.

The gate refuses missing, changed, or unreviewed inputs before invoking the
pipeline. `--validate-only` checks the manifest and media without inference.
`python3 -m unittest examples/transcribe/test_verify_youtube_pipeline.py` runs
the focused pure validation tests on a development host. They are not native
transcription evidence.
