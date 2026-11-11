---
id: 01a1c509-1a2b-7a01-8c03-2718c1f34101
seq: 19
form: page
type: runbook
title: Make a PDF from slides or images
description: 'Cross-platform steps to export pages as images, make and combine PDFs, and turn PDF pages back into video-ready PNGs with media — WHEN a member needs a PDF deliverable from a slide or document app (keywords: media, pdf, slides, images, png, combine, pdfcraft, ffmpeg, agg)'
tags:
    - media
    - pdf
    - slides
    - cross-platform
status: candidate
evidence: 'Sprint 603 M3 2026-11-10: commands and prerequisites checked against examples/media/media.bsh and its skill guide'
source:
    tool: codex
created: "2026-11-10T20:00:00Z"
---

## Install once

Use the line for this computer, then run `media doctor`. Doctor downloads and SHA-256 verifies the pinned `pdfcraft-cli`, so it needs network access the first time.

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

## Make the PDF

1. In the member's own slide or document app, export every slide/page as PNG or JPEG. Use one size and sortable names such as `01-title.png`, `02-plan.png`. This is the only app-specific step.
2. Compose the plain-file verbs:

```sh
media pdf 01-title.png 02-plan.png -o deck.pdf --title "Project plan" --author "Project owner"
media combine deck.pdf appendix.pdf -o complete.pdf
media slides complete.pdf slides/       # slides/01.png, 02.png, ... for a video
```

3. Open `complete.pdf` in any browser or PDF viewer, check page order and metadata, then share that file with the other members. `media` itself opens no browser and needs no camera or microphone. If this PDF later becomes a narrated video, the `record` page asks the member to allow camera/microphone once and press **Start**, **Stop**, then **Use this**; the PDF steps remain unchanged on every OS.

The reusable verbs are `pdf` (images to PDF), `combine` (PDFs to one PDF), and `slides` (PDF pages back to numbered PNGs). Change or repeat only the verb whose output needs work.
