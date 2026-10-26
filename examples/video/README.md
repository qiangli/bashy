# examples/video — make a demo video with bashy, locally

A storyboard in, one MP4 out. Title cards, a webcam take, and real terminal
sessions recorded while bashy runs them, typed at a human pace, side by side
when two terminals tell the story, with your voice on top. Everything runs
on your own machine; nothing is uploaded.

This is an example, not a bashy command: it composes tools you install
yourself.

| tool | role | license |
|---|---|---|
| [asciinema](https://asciinema.org) 3.x | record each step headless | GPL-3.0 (run, not bundled) |
| [agg](https://github.com/asciinema/agg) | render a cast to frames | Apache-2.0 |
| [ffmpeg](https://ffmpeg.org) | encode, stack, concatenate, mux audio | LGPL/GPL (run, not bundled) |
| bashy | runs every recorded step | MIT |

```bash
bashy dag examples/video doctor                      # check the tools
bashy dag examples/video sample                      # build sample/build/demo.mp4 (about 30 s)
bashy dag examples/video build PROJECT=$PWD/my-demo  # your project, from anywhere
```

Naming the folder (`bashy dag examples/video …`) runs the recipe inside it, so
call it from anywhere and pass `PROJECT` as an absolute path.

## Who does what

- **An agent** writes the project (storyboard, cards, step scripts), runs the
  build, checks `build/demo.mp4` (duration, frames via `ffprobe`), and fixes a
  failing step by editing its script. Everything is a file and a command.
- **A human** asks in chat ("make a 2-minute demo of X"), reviews the MP4,
  and records only what needs a person: the camera take and the narration.

## A project

```
my-demo/
  storyboard.txt     the order of segments
  cards/title.txt    first line bold, the rest centred under it
  steps/run.sh       a recorded terminal step
  camera/intro.mov   optional: a webcam take (or record it live, below)
  narration.m4a      optional: your voice over the whole video
```

`storyboard.txt`, one segment per line:

```
card   title 4          # cards/title.txt for 4 seconds
camera intro 60         # camera/intro.mov, or record webcam + mic for 60 s
step   run              # record steps/run.sh
pair   start resume     # two steps side by side: the right one waits
                        # until the left one has finished
card   close 4
```

A step is a script sourced into a bashy session with three helpers:

```bash
say an ordinary Bash script, unchanged     # a dim caption line
show "ls -1 | wc -l"                       # typed, then really run
pause 2                                    # hold the frame
```

Output is real: `show` types the command and then executes it. If a command
fails, the recording shows the failure, so rehearse first.

## Knobs

`VIDEO_COLS`/`VIDEO_ROWS` (100x24, a 16:9 frame), `VIDEO_PAIR_COLS`/`ROWS`
(64x30), `VIDEO_FONT_SIZE` (28), `VIDEO_THEME` (monokai), `VIDEO_IDLE` (4 s
cap on pauses; keep it above the longest caption hold), `TYPE_DELAY` (0.012 s per character), `SAY_WPM` (240: captions are held for their reading time), `VIDEO_CAMERA` (`0:0`,
see `bashy dag cameras`), `VIDEO_SHELL` (bashy).

## Notes from making real videos with it

- Odd frame sizes: agg can produce an odd width; H.264 needs even sizes, so
  every render is padded.
- Recorders and ffmpeg read stdin; inside a `while read` loop over the
  storyboard they swallow lines unless stdin is closed (`</dev/null`,
  `-nostdin`).
- Typing delays use an in-shell `read -t` on an idle pipe instead of one
  `sleep` process per character.
- Recordings set `BASHY_TELEMETRY_QUIET=1` and `BASHY_HINTS=off` so only the
  step's own output appears.
