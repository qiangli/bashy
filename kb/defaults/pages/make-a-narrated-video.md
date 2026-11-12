---
id: 01a1c509-1a2b-7a02-8c03-2718c1f34102
seq: 20
form: page
type: runbook
title: Make a narrated video
description: 'Cross-platform project layout and verbs for a narrated MP4 (own voice or an expressive local neural voice) from slides, a deck PDF, camera and terminal scenes, with captions — WHEN an owner or member needs a demo, pitch or one-minute intro video that can move between Windows, macOS and Linux (keywords: media, record, video, narration, voice, tts, pitch, intro, deck, slides, camera, cast, agg, ffmpeg, captions, storyboard)'
tags:
    - media
    - video
    - narration
    - voice
    - cross-platform
status: candidate
evidence: 'Sprint 603 M3 2026-11-10: workflow checked against examples/media and examples/record skill guides plus examples/video/README.md. 2026-11-12: a one-minute pitch intro made from a deck PDF with an expressive local neural voice (now media voice), accepted by its owner; media test.sh covers media voice with a fake model'
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

`storyboard.txt` orders `card`, `slide`, `step`, `pair`, and `camera` segments. Put a Bashy scene in `steps/NAME.sh`; the build records it with the registered `cast` command, then agg renders it. Put one spoken line per segment in `narration.txt`. One segment per line, paths relative to the project:

```text
slide slides/01.png 18    # image, seconds (default 3)
step demo                 # runs steps/demo.sh, recorded by cast
slide slides/02.png 18
```

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

## Draft first, final once

Iterate on cheap drafts, and spend the expensive resource once. The OS voice is instant and free, so use it to settle everything except the voice: the script, the slides, the timing and the captions.

```sh
media narrate demo --voice default && media build demo    # draft: review, edit, repeat
```

Show each draft to the person and change only what they ask. When they accept the draft, make the final voice once: record the takes (`record audio`), or run `media voice` (a long local job), or use a paid or rate-limited voice service if the person has one. Then `media narrate demo --own` and `media build demo`. If one line is wrong in the final, regenerate or re-record only that line, not the whole set.

## Slides from a deck

A pitch or intro video is often just the deck, narrated. Export the deck as a PDF, then:

```sh
media slides deck.pdf demo/slides     # demo/slides/01.png, 02.png ...
```

Pick the pages that carry the story and list them in `storyboard.txt` (`slide slides/07.png 9`). Keep one narration line per slide; for a one-minute intro, about six slides of 8 to 11 seconds and 20 to 30 words each.

## An expressive voice without recording

When the person does not want to record, `media voice` writes the takes with a local neural model (Chatterbox, MIT), so the result is far livelier than an OS voice and nothing leaves the machine:

```sh
media voice demo                      # demo/voice/01.wav ... ; then:
media narrate demo --own
```

- Write the lines for the ear: short sentences, an exclamation where the energy should lift. Put pronunciation respellings (a product name, initials such as `D H N T`, numbers as words) in `narration-spoken.txt`, same line count; `narration.txt` stays the caption text.
- `--energy` sets how animated it sounds: 0.5 calm, 0.7 upbeat (default), 0.9 very animated.
- The default speaker is synthetic. `--reference me.wav` uses a sample of the person's own voice instead (record one with `record audio`). Never use another person's voice.
- The first run needs `uv` (`media doctor` prints the install line) and downloads a few GB of pinned packages and model weights into the bashy cache. It runs on CUDA, Apple MPS or CPU. That one-time setup can take a while; an agent can run it in the background (for example as a `bashy foreman` goal) and carry on.
- Listen before shipping. A line that sounds wrong: fix its respelling and rerun `media voice demo --force`, or replace only that `voice/NN.wav` with a recorded take.
- Say it is synthetic: end the captions with a line such as "narration: text-to-speech, script by the owner".

## Narrate, build, share

```sh
media narrate demo --own
media build demo
```

For a quick OS voice draft, run `media narrate demo --voice default` before `media build demo`. OS voices sound flat; replace the draft with `--own` takes for the final version, recorded or generated (next section).

The result is `demo/build/demo.mp4`. If `demo/captions.srt` is present, the build uses agg caption cards with ffmpeg `overlay` to burn captions into the picture and also muxes them as a soft `mov_text` subtitle track. View the MP4 locally and share the file.

To hand the project to another member on any OS, share the whole folder. Keep paths inside `storyboard.txt` relative; CRLF and LF are both accepted and normalized. The receiving member runs `media doctor`, then the same `media narrate` and `media build` verbs without rewriting paths.
