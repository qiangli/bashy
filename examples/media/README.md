# examples/media — PDF and video, identical on macOS, Windows and Linux

`media.bsh` is a Bash# skill-script (one file: the command plus its
`~~~skill as guide`, like `examples/record/record.bsh`). Register it once with
`bashy commands add`, or run `bashy examples/media/media.bsh VERB`.

| verb | does |
|---|---|
| `media doctor` | downloads the pinned `pdfcraft-cli` (SHA-256 verified, bashy cache); checks ffmpeg (H.264 encoder + `overlay` filter), ffprobe, agg, a recorder, a voice; exits 1 printing the brew / winget / apt line for anything missing |
| `media pdf IMG... -o OUT.pdf [--title T --author A]` | images to PDF |
| `media combine A.pdf B.pdf... -o OUT.pdf` | merge PDFs |
| `media slides IN.pdf DIR` | `DIR/01.png`, `02.png`... for the video |
| `media voice PROJECT [--energy 0.7] [--reference FILE] [--force]` | `PROJECT/voice/NN.wav`: an expressive local neural voice (Chatterbox, MIT; synthetic speaker or the person's own sample) for `narrate --own`; first run needs `uv` and downloads pinned packages and weights |
| `media narrate PROJECT --own \| --voice NAME` | `PROJECT/narration.m4a`: line N of `narration.txt` over segment N, fitted, -16 LUFS |
| `media build PROJECT` | `PROJECT/build/demo.mp4` through `examples/video/video.sh` |

Each verb prints only the paths it wrote. `--own` uses the member's own
recordings `PROJECT/voice/NN.*` (make them with `record audio --script`; the
verb prints the exact command for each missing one). `--voice` is the OS voice
(macOS `say`, Windows System.Speech, Linux `espeak-ng`) as a draft aid only.
`step` segments use `cast record` when `cast` is present, else asciinema; agg
always uses the OFL IBM Plex Mono shipped in `fonts/`, so cards and captions
look the same everywhere. No browser is used.

Shared projects accept CRLF or LF in `storyboard.txt`, `narration.txt` and
`cards/*.txt` and are rewritten as LF; paths inside are relative; `sample/`
ships a `.gitattributes`.

## Pins and licences

pdfcraft-cli 0.20.0 (MIT OR Apache-2.0): macOS `pdfcraft-cli-0.20.0-macos-universal.zip`
(one file for arm64 and amd64), Linux `pdfcraft-cli-0.20.0-linux-{x86_64,aarch64}.tar.gz`,
Windows the portable zip `pdfcraft-0.20.0-windows-{x64,arm64}-portable.zip`
(it carries `pdfcraft-cli.exe`; there is no CLI-only Windows asset). ffmpeg and
agg are never downloaded or bundled (GPL). The font is OFL (`fonts/OFL.txt`).

## Test

```bash
bash examples/media/test.sh   # pdf, combine, slides round trip, narrate + build, voice (fake model)
bash examples/video/test.sh
```
