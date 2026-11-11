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

```sh
# macOS
brew install ffmpeg agg
# Windows (PowerShell; either agg line works)
winget install Gyan.FFmpeg
winget install asciinema.agg    # or: scoop install agg
# Debian/Ubuntu Linux
sudo apt install ffmpeg
cargo install --locked agg      # or install agg's release binary
media doctor
```

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

`storyboard.txt` orders `card`, `slide`, `step`, `pair`, and `camera` segments. Put a Bashy scene in `steps/NAME.sh`; the build records it with `cast record` by default (`bashy cast record` if it is not on PATH), using asciinema only as a fallback, then agg renders it. Put one spoken line per segment in `narration.txt`.

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

The result is `demo/build/demo.mp4`. If `demo/captions.srt` is present, the build uses agg caption cards with ffmpeg `overlay` to burn captions into the picture and also muxes them as a soft `mov_text` subtitle track. View the MP4 locally and share the file.

To hand the project to another member on any OS, share the whole folder. Keep paths inside `storyboard.txt` relative; CRLF and LF are both accepted and normalized. The receiving member runs `media doctor`, then the same `media narrate` and `media build` verbs without rewriting paths.
