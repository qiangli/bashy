---
id: 01a1c509-1a2b-7a02-8c03-2718c1f34102
seq: 20
form: page
type: runbook
title: Make a narrated video
description: 'Cross-platform project layout and verbs for an own-voice narrated MP4 with camera, terminal scenes and captions — WHEN an owner or member needs a teachable demo video that can move between Windows, macOS and Linux (keywords: media, record, video, narration, camera, cast, agg, ffmpeg, captions, storyboard)'
tags:
    - media
    - video
    - narration
    - cross-platform
status: candidate
evidence: 'Sprint 603 M3 2026-11-10: workflow checked against examples/media and examples/record skill guides plus examples/video/README.md'
source:
    tool: codex
created: "2026-11-10T20:00:00Z"
---

## Install once

Register the examples with `bashy --bashsharp examples/defaults/bashy-defaults.bsh apply` from the source checkout (or `bashy-defaults apply` after installing Bashy's source pin). This registers `media`, `cast`, and `record` with declared effects. `media doctor` registers ffmpeg, ffprobe, agg, and verified PdfCraft.

```sh
# macOS
brew install ffmpeg agg
# Windows (PowerShell; either agg line works)
winget install Gyan.FFmpeg --accept-source-agreements --accept-package-agreements
winget install asciinema.agg --accept-source-agreements --accept-package-agreements
# Debian/Ubuntu Linux
sudo apt install ffmpeg
cargo install --locked agg      # or install agg's release binary
media doctor
```

If winget's source fails, download the ZIP from [FFmpeg's Windows essentials builds](https://www.gyan.dev/ffmpeg/builds/) and the Windows `agg.exe` from [asciinema/agg releases](https://github.com/asciinema/agg/releases). Extract them under a folder you own, and set `MEDIA_FFMPEG`, `MEDIA_FFPROBE`, and `MEDIA_AGG` to the full executable paths before `media doctor`. Doctor registers those names. On Windows, doctor also provisions Go for the cast recorder and uses the pinned PowerShell 7 fence for System.Speech.

## Project and takes

Keep the project as portable plain files:

```text
demo/
  storyboard.txt
  narration.txt
  slides/
  steps/
  camera/
```

`storyboard.txt` orders `card`, `slide`, `step`, `pair`, and `camera` segments. Put a Bashy scene in `steps/NAME.sh`; the build records it with the registered `cast` command, then agg renders it. Put one spoken line per segment in `narration.txt`.

Run `media narrate demo --own` once. It creates `build/script/NN.txt` and prints the missing command for each line. Run those commands from the project folder, for example:

```sh
cd demo
record audio --script build/script/01.txt --out voice/01.m4a
```

Tell the member a local recording page will open. They allow microphone access once, press **Start**, speak the displayed line, press **Stop**, review or retake it, then press **Use this**. Repeat for each printed command. For a `camera intro` segment, prepare a short script and record the take:

```sh
record video --script camera/intro.txt --out camera/intro.mp4
```

The same local page asks for camera and microphone access once; the member presses **Start**, **Stop**, and **Use this**. Nothing is uploaded.

## Narrate, build, share

```sh
media narrate demo --own
media build demo
```

For a quick OS voice draft, run `media narrate demo --voice default` before `media build demo`. Replace the draft with `--own` takes for the final version.

The result is `demo/build/demo.mp4`. If `demo/captions.srt` is present, the build uses agg caption cards with ffmpeg `overlay` to burn captions into the picture and also muxes them as a soft `mov_text` subtitle track. View the MP4 locally and share the file.

To hand the project to another member on any OS, share the whole folder. Keep paths inside `storyboard.txt` relative; CRLF and LF are both accepted and normalized. The receiving member runs `media doctor`, then the same `media narrate` and `media build` verbs without rewriting paths.
