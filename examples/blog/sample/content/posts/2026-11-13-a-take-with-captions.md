---
title: "A take with captions"
date: 2026-11-13
video: media/demo.mp4
captions: media/demo.vtt
audio: media/note.wav
summary: A short recorded take with captions attached — video from the screen, a voice note, and a .vtt transcript the player shows natively.
---

Some things are easier said than shown. This post carries a take: a short
video with a caption track, and a voice note recorded the same way.

The captions are a plain WebVTT file next to the media:

```text
WEBVTT

00:00.000 --> 00:00.800
A one-second test pattern,

00:00.800 --> 00:01.000
built and captioned locally.
```

Captions can be written by hand or generated on this machine by the local
transcription pipeline — the command asks for it explicitly and nothing
leaves the host either way.

## Why a take belongs in a journal

- A demo is evidence. The video is the artifact, not a description of it.
- A voice note ages better than a paragraph of hedging.
- Captions make both searchable, and keep the page usable with the sound off.

The take above is a placeholder pattern, but the plumbing is real: record,
attach, caption, publish.
