# examples/clip — `clips`: async screen and video clips

The command registers as **`clips`**: bashy already ships a `clip` skill
(the clipboard), and a registered command never shadows a command bashy
ships. The job, directory and module stay `clip`.

`clip` queues a short clip — a segment cut from a video, a terminal scene
rendered to a looping gif or mp4, a screen recording — and returns at once
with a job id. A work command renders queued jobs in the foreground of your
choosing, finished clips land in a library directory, and one loopback page
shows the queue and plays the clips. It composes the media family: scenes
come from [cast](../cast) (or asciinema), takes from [record](../record),
storyboards from [video](../video), publishing from [vlog](../vlog).

Like every example here it is a skill-tier command, never a core change:
one Bash# file plus a small Go server, registered once.

```bash
bashy dag examples/clip test      # module tests + the script end to end
bashy dag examples/clip register  # register it as the command `clips`
bashy dag examples/clip use       # render the sample scene with real tools
```

After `register`:

```bash
clips queue --from takes/intro.mp4 --start 12 --end 18 --name intro
clips work
clips status
clips take c1a2b3c4                     # prints the clip's path; exit 3 while waiting
clips queue --from scene.cast           # a terminal scene -> looping gif
clips queue --from scene.cast --format mp4 --start 1 --end 3
clips queue screen --seconds 10         # the screen (Linux X11 this sprint)
clips serve                             # the library page on loopback
```

## The asynchronous part

- `queue` validates and writes one JSON job file (`jobs/<id>.json`); nothing
  renders, nothing blocks, the id is printed at once.
- `work` renders queued jobs one at a time — `ffmpeg` for video and the
  screen, `agg` for terminal scenes — marking each running → done (with
  its artifact) or failed (with the reason). A failed job never stops the
  queue; a worker that dies leaves its job running, and the next `work`
  re-queues it after ten minutes.
- `take ID` composes: exit 0 prints the finished clip's path, exit 3 means
  still queued or rendering, exit 1 failed (`ffprobe "$(clips take ID)"`).
- `serve` is the library page: the queue with live statuses, finished clips
  playing inline, and a form that queues from the page. The page only reads
  and writes the data directory — the console runs shaped app pages with
  read/write effects only — so rendering stays with `clips work`, which the
  page names while clips are waiting.

## Anatomy

| Piece | Job |
|---|---|
| `clip.bsh` | the command: a `~~~skill` fence (what agents read) and the shell half that builds and runs the Go binary |
| `server/` | the engine: `job.go` (the job store, one JSON file per clip), `worker.go` (the render queue with injectable tools), `page.go` (the library page), `main.go` (the CLI verbs) |
| `go.mod` | this example's own module, standard library only |
| `checks/` + `contract.bsh` | the shaping verifier's check: build and test the module in the isolated clone |
| `shaped.json` | the shaped-app manifest: the console builds `./server` and runs it with `$ADDR`/`$DATA` |
| `clip_bsh_test.go` | drives `clip.bsh` through a built bashy end to end, with a stub `ffmpeg` (skips in a vendored clone) |
| `sample/scene.cast` | a tiny asciicast v2 scene for `bashy dag examples/clip use` |

Jobs are plain files under one data directory — `jobs/` for the queue,
`clips/` for the artifacts — so the CLI and the page share one store with
no daemon between them. `CLIP_DATA` selects it (default `$BASHY_APP_DATA`
inside a shaped app, else a `clip` directory under the Bashy home).

## Tools and platforms

`ffmpeg` cuts video and records the screen; `agg` renders terminal scenes;
`clips doctor` checks both. They run, never bundle: the same arrangement as
[video](../video). Screen capture is an X11 job (Linux, this sprint's
scope); on other platforms a screen job fails with the reason instead of
hanging. Everything else works wherever ffmpeg/agg do.

## Shaping

`clips` is a shapeable app: `bash register.sh` registers the command, the
skill and the shaped app (the public `bashy app add --type shaped` path).
Then wish a change — at `/shape/clips/` on the console or
`bashy shape wish clips "..."` — verify, preview, approve it for yourself,
and undo. `shaped.json`'s genie examples seed the wish form.

## Verification

```sh
GOWORK=off go -C examples/clip test ./...   # module + script tests
bashy dag examples/clip test                # the same through the task graph
bashy dag examples/clip use                 # a real render with host tools
```
