---
name: video
description: bashy dag front door for examples/video — check tools, build a storyboard into one MP4, clean
type: dag
default: sample
vars:
  - PROJECT ?= sample
---

# video — task graph

Turns a storyboard of title cards, camera takes and recorded terminal steps
into one MP4, entirely on your machine. `video.sh` does the work by composing
three host tools: asciinema records each step (the step runs under bashy),
agg renders the cast, ffmpeg encodes, stacks, concatenates and adds a voice
track. Nothing is uploaded. `PROJECT=path/to/project` picks another project
directory; see README.md for its layout.

## Tasks

### doctor
Check that asciinema, agg, ffmpeg, ffprobe and bashy are on PATH.
Effects: read, exec

```bash
./video.sh doctor
```

### cameras
List the macOS camera and microphone devices (for VIDEO_CAMERA).
Effects: read, exec

```bash
./video.sh cameras
```

### sample
Build the bundled 30-second sample into sample/build/demo.mp4.
Requires: doctor
Effects: read, write, exec
Generates: sample/build/demo.mp4

```bash
./video.sh build sample
```

### build
Build any project directory: `bashy dag build PROJECT=../my-demo`.
Requires: doctor
Effects: read, write, exec

```bash
./video.sh build "$PROJECT"
```

### clean
Remove a project's build directory.
Effects: write

```bash
./video.sh clean "$PROJECT"
```
