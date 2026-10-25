# S503 native transcription CI gate

`.github/workflows/s503-native-transcription.yml` runs on the six public native
runners after the reviewed admission record is pushed to
`s503-contract-integration`, or by manual dispatch. It cannot turn green from
an unreviewed build or a media download. The admission record
`docs/sprint-503/evidence/native-transcription-admission.json` is absent, so
all six jobs remain blocked.

## Admission required

The `s503-native-transcription-admission-v2` record must bind:

- The six attested whisper binaries from run `40195543786`, exact per-runner
  SHA-256 values, and source commit `c59c3aa134588e21e1039750515fc00159cc2cb6`.
- Deno 2.9.8 archive URLs and archive/executable hashes for each runner, the
  yt-dlp v2026.10.21 zipapp hash with bundled EJS v0.8.0, and the ggml-tiny
  model hash and byte count. `docs/sprint-503-deno-admission.md` records the
  Deno and EJS work. The native job pins CPython 3.12.15 on Linux and 3.12.10
  on macOS and Windows, records its executable hash, and passes that exact path
  to the source adapter.
- Independently acquired canonical and shortened operator URL media hashes,
  499-second duration, a human-reviewed excerpt, score tolerance, and review
  evidence. Equal media bytes are allowed only after separate acquisitions.
- A separate licensed controlled speech WebM at least five seconds long, with
  a reviewed transcript excerpt of at least six words, duration, SHA-256,
  byte count, license and review evidence. Its artifact must be named
  `s503-controlled-speech`, contain only `speech-long.webm`, and be identified
  by an exact successful GitHub run ID and source commit. The current two-second
  `examples/transcribe/webm-opus/testdata/speech.webm` is a codec fixture and
  is explicitly rejected as the controlled speech case. A longer CC0
  Wikispeech remux is a candidate, pending native transcript review and
  publication as the pinned artifact.
- Six reviewed offline enforcer executable hashes and methods: Linux
  `linux-unshare-net`, macOS `macos-sandbox-deny-network`, Windows
  `windows-firewall-program-block`. Each probe is a real Python TCP attempt to
  `1.1.1.1:443`.

No admission record or controlled speech artifact is created by this workflow.

## What each native job does

Each runner checks native OS, architecture, Go and Python identity; builds
candidate Bashy, seed helper and pure-Go WebM/Opus decoder; downloads its
whisper artifact from the pinned run through `actions/download-artifact`; and
verifies executable SHA-256, build report, source archive, identity manifest
and GitHub attestation. It downloads and verifies all other dependencies and
the controlled speech artifact before any inference. The model path remains in
the verified `models/whisper` cache.

The online phase invokes `source_youtube.py` separately for the canonical and
short URLs, retains each WebM in a separate work directory, and compares the
bytes and media identity with the reviewed admission. It writes the exact
candidate-bound private manifest and runs `verify-youtube-pipeline.py --online`.
That harness exercises script and skill paths, controlled and operator
TXT/SRT/VTT, both URL forms, reviewed transcript excerpts, Bash# function
receipts, failure cleanup and native process-group cancellation. Its online
mode also interrupts an admitted, rate-limited yt-dlp download after a partial
file appears, requiring a structured cancellation and complete cleanup. The job
then proves the same runner can connect to `1.1.1.1:443` before isolation.

The offline phase runs the harness again on retained media under an OS policy
external to Bashy. Linux enters `unshare --net` with `sudo`; macOS invokes
`sandbox-exec` with `deny network*`; Windows installs temporary outbound block
rules for Python, Bashy, the helper, decoder, whisper, Deno, Bash, PowerShell
and curl, checking active rules and removing them in `finally`. The OS
policy's executable SHA-256 must match the admission. A child Python process
tries a real TCP connection before and after the offline harness while the
policy is active. A successful connection, missing rule, unsupported namespace
or sandbox, wrong error, or failed replay fails the job. Windows avoids a
global outbound block that could sever the hosted runner's own heartbeat.
Its canary accepts only Winsock 10013 (access denied), observed with an active
program rule; a timeout or unreachable network is not firewall proof.
The artifact upload retains manifests, native identities, hashes, transcripts,
receipts and canary reports even when the job fails.

## Remaining gates

The workflow has been syntax checked locally; no local builds, inference or
OCI runs were performed. Actual hosted-runner execution must establish whether
Linux permits `sudo unshare --net`, whether macOS `sandbox-exec` behaves on both
GitHub images, and whether Windows program rules deny the canary on x64 and
ARM64 without disrupting the runner. Any unsupported target stays red.

The final job requires `pass` for online download cancellation and local
pipeline cancellation in both modes. Offline replay records
`not_tested_offline` for download cancellation while network access is denied.
The Windows CTRL_BREAK_EVENT path and the controlled partial-download probe
still need native execution on all six runners. Until those results, the
required fixture and operator review, and all six artifacts are inspected,
S503.8 remains **0/6 full native passes**.
