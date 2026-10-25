# Controlled native speech fixtures

`spanish-proverb.wav` is an authored repeat of the 2.248-second recording
“de todo hay en la viña del Señor” by Rodrigo5260. The [primary Wikimedia
Commons file page](https://commons.wikimedia.org/wiki/File:LL-Q1321_(spa)-Rodrigo5260-de_todo_hay_en_la_vi%C3%B1a_del_Se%C3%B1or.wav)
publishes the recording under CC0-1.0. The original WAV is 215,852 bytes,
with Rodrigo5260 credited as speaker and recorder (own work, recorded
2022-07-15 in Commons image metadata). It is
SHA-1 `08acfe2ddfe734c5c3f8b35ead5f9bed84f1e315`, SHA-256
`bf2168307813beb1037c3ee4e9efceee62bfbd0e4920adcc09f74d03e4b5118b`.
`generate_spanish.py` verifies those bytes and the 48 kHz mono PCM16 format,
then writes three unmodified copies of the PCM with 500 ms of silence between
them using Python's standard `wave` module. The resulting 743,456-byte WAV has
SHA-256 `df23f58c95b54215e136bd6b1865a9aba360010a02822ec56e8854bc8074e2bb`.
The authoring script is not invoked by the native gate.

`speech-video.webm` is authored on the hosted `ubuntu-24.04` runner by
`.github/workflows/s503-author-controlled-video.yml` from the already pinned
CC0 [Wikispeech source](https://commons.wikimedia.org/w/index.php?title=File:Wikispeech-sample2.opus&oldid=1042404750)
audio fixture. The workflow adds an authored black VP9 video stream and copies
the Opus speech stream without re-encoding. Its raw evidence records the exact
FFmpeg command, version, source SHA-256, output SHA-256, duration and VP9/Opus
stream identities. FFmpeg is an external fixture-authoring tool on hosted CI;
the runtime gate builds and registers the pure-Go WebM/Opus decoder only.
The successful authoring run is `40199530099`, with 49,475-byte video SHA-256
`36c7fb05a66ec3f29d498fb0c97b8b72de0e9243092b51d2f34e02751bcc93de`.
The `ffprobe` report records an 11-second container with VP9 video and Opus
audio. The audio stage retains the original 10,660 ms speech packet span.

The native gate validates each committed media file against
`controlled-native.json` before running the Bash# script and skill paths.
The expected text excerpts are from the two primary source descriptions, not
from a fabricated inference result. Every target must produce its own real
transcript and automatic language report to pass.
