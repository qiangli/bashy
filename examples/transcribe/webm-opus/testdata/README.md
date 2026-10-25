# Controlled non-silent fixtures

`speech.webm` contains the first 100 audio packets (2 seconds before pre-skip)
of **Wikispeech-sample2.opus**, authored by Sebastian Berlin (WMSE), 2017-02-15.
The original is synthesized speech describing Wikispeech; no model/inference
is executed in these tests. The Opus packets are remuxed unchanged into an
authored finite WebM, preserving mono OpusHead and 312-sample pre-skip.

- Source/license: https://commons.wikimedia.org/wiki/File:Wikispeech-sample2.opus
- Stable source page: https://commons.wikimedia.org/w/index.php?title=File:Wikispeech-sample2.opus&oldid=1042404750
- Download: https://upload.wikimedia.org/wikipedia/commons/2/2f/Wikispeech-sample2.opus
- File author dedicates it under CC0-1.0: https://creativecommons.org/publicdomain/zero/1.0/
- Original 47,030-byte file SHA-1: c8e4fd2be6992f0764546cd5fb3d1fc03702e4aa
- Original SHA-256: 9744230eb367b00f1aa5d0c64572c24b2cc533d5f893294bcaabdd912ed1b5a3

`stereo.webm` contains an authored 0.6-second stereo signal: left 440 Hz at
amplitude 9000, right 880 Hz at amplitude 5000, encoded from 48 kHz PCM16 with
libopus 1.6.1. The signal and generated test artifacts are dedicated CC0-1.0.
The two different channels test real codec downmixing, not a mono packet with
stereo metadata. Both fixtures are short development unit-test inputs.

`*-reference.pcm` is independent little-endian 16 kHz mono PCM16 from libopus
1.6.1, with the 104 output pre-skip samples removed. The decoder gate compares
sample count, nonzero energy and SNR against these bytes; it is not a transcript
or a human listening-quality claim. At least half the samples must be nonzero,
reference RMS must exceed 100, and SNR must exceed 20 dB.

`manifest.json` pins every binary fixture/reference SHA-256. `generate.py`
records the reproducible packet-remux/authoring/reference procedure. It needs
an **existing** libopus development library through `LIBOPUS`; it installs
nothing and is not called by tests/builds. The upstream download is checked
against the published SHA-1. It used an already installed BSD-licensed
libopus 1.6.1 for fixture authoring only; no native inference was run.
The actual test/runtime dependency remains pure-Go MIT Pion v0.1.0.

Malformed, cancellation, video coexistence, lacing, trim, and limit fixtures
are authored in `adapter_test.go`; they do not rely on downloaded media.
