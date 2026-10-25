# Sprint 503 Linux musl native inference experiment

Story #2742 (`74ed924ba244`). The successful two-target static musl build run
`40220541369` established build, link-input, ELF and CLI-help evidence. It did
not exercise speech recognition. The manual
`.github/workflows/s503-linux-musl-runtime-probe.yml` now adds an evidence-only
native CPU inference replay after that build and its binary attestation. Each
Linux amd64/arm64 job remains capped at 60 minutes; each CLI process has a
480-second limit and is killed with its process group and reaped on timeout or
Python cancellation. No product admission is made by this workflow.

The gate uses the **same retained executable bytes** whose SHA-256 and static
ELF report were produced by `probe_linux_musl_runtime.py`. It downloads the
reviewed multilingual `ggml-tiny.bin` from the pinned Hugging Face revision and
checks SHA-256 `be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21`
and 77,691,713 bytes before inference. The controlled fixture index
`examples/transcribe/fixtures/controlled-native.json` pins the two CC0 source
media bytes, license pages, durations, expected language, transcript excerpts
and score thresholds. English uses the 10.66-second Wikispeech WebM; Spanish
uses the 7.744-second authored proverb WAV. Hosted FFmpeg converts each verified
input to mono 16 kHz PCM16 WAV, checks duration and retains source/conversion
hashes, version and exact command. FFmpeg is a hosted fixture preparation tool,
not a linked or distributed CLI input.

Both cases run the retained CLI directly with `--language auto --no-gpu
--output-json`, matching the registered adapter's CLI contract. The gate
requires exit zero, a real JSON result with `en` or `es` detected language,
nonempty timed segments within media duration and the existing controlled
reference threshold. It retains the full transcript JSON, CLI log, elapsed
time, binary/model/fixture hashes and summary per case. An online baseline
establishes external TCP connectivity before any namespace restriction.

The same cases run a second time under `sudo /usr/bin/unshare --net`. The
reviewed S503 Linux enforcer SHA-256 is checked against
`native-transcription-admission.json` before entry. The process verifies that
its `/proc/self/ns/net` identity differs from the host, then reuses the
`s503-offline-enforcer-probe.py` socket probe before and after both inferences.
Each probe must show two live OS-denied `1.1.1.1:443` connection attempts.
All model/media setup finishes before the namespace is entered. The workflow
retains partial results and per-action failure attempts. `inference-result.json` is written
only after every source hash, transcript, online baseline and offline denial
receipt passes; that result still has `product_admitted: false`.

The new probe has three local policy tests for transcript validation and
bounded child cleanup. Native recognition, host FFmpeg conversion and Linux
network isolation are deliberately tested only on the two GitHub-hosted
runners. A passing future run is still subject to independent notice and
source-closure review, release packaging/artifact review, and supported host
and product dependency policy review before admission.
