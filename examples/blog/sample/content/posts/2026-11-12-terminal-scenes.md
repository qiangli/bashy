---
title: "Shaping a build, part 2: the terminal scene"
date: 2026-11-12
series: shaping-notes
part: 2
cast: media/build.cast
summary: A recorded terminal session embedded as a post — the cast family records it, the journal renders it as a readable transcript.
---

A build log tells you what happened; a terminal recording shows you how it
looked. This post embeds one: recorded under a pseudo-terminal by the cast
family, in the open asciicast v2 format, and rendered here as a plain
colored transcript — no player download, no script from anywhere.

The scene below is a real build of a site like this one:

The recorder captures colors and cursor movement as the terminal printed
them, so a green `ok` is the same green on the page. Because the file is
just JSON lines, the transcript is diffable, greppable and small:

```go
// a cast file is one JSON header ...
{"version": 2, "width": 80, "height": 24, "title": "build"}
// ... then one event per output chunk
[1.204, "o", "\u001b[32mblog: 3 post(s) -> public\u001b[0m\n"]
```

## Captions, not transcripts of everything

For voice and video takes the journal takes a caption file instead — a
`.vtt` beside the media, written by hand or by local transcription. The
player shows it natively:

```bash
blog captions content/posts/my-take.md   # transcribe locally, attach .vtt
```

The next post has one attached to a real take.

That closes the loop from part 1: wish, candidate, verify, preview, approve,
undo — recorded, embedded, and readable here.
