# examples/blog — a personal blog or wiki with rich media

A journal you author and host yourself: plain Markdown posts that carry the
media the bashy families already produce — video and voice takes
([record](../record)), recorded terminal scenes ([cast](../cast)), locally
generated captions ([transcribe](../transcribe)) — plus diagrams, colored
code and math for technical notes. Built on the same base as
[examples/site](../site) (a Hugo-compatible source tree and the bundled
admin CMS); no new fork. Everything renders locally; nothing phones home.

```bash
bashy dag examples/blog build                     # sample/ -> sample/public/
bashy dag examples/blog serve                     # console tile: http://127.0.0.1:22749/blog/
bashy dag examples/blog register                  # register the command + skill + shaped app
bashy dag examples/blog shape-demo                # full shaping lifecycle demo
```

Use your own journal with `SITE=$PWD/my-journal`. The same commands work
wherever bashy does.

## A journal

```
my-journal/
  hugo.toml                    title, tagline, author
  content/
    _index.md                  home intro
    about.md                   standalone page (the wiki half)
    posts/2026-11-10-notes.md  one post per file, newest first by date
  media/                       takes, casts, captions (copied to public/media/)
  static/                      css and the admin CMS (served as-is)
  generator/                   the self-contained pure-Go builder
  checks/smoke.bsh             declared verifier checks
  contract.bsh                 shaping effects contract
```

A post's front matter attaches its media:

```markdown
---
title: A take with captions
date: 2026-11-13
series: shaping-notes
part: 2
video: media/demo.mp4      # a take; captions: media/demo.vtt rides along
audio: media/note.wav      # a voice note
cast: media/build.cast     # a terminal scene, rendered as a transcript
summary: One line for the home page.
---
```

The body renders fenced ```bash / ```go ... code with syntax coloring,
```diagram edge lists as SVG, ```math blocks and $inline math$ — all pure
HTML/CSS, no remote fonts or scripts.

## Rich media from the families

- `bashy blog post "Title" [--series S]` — new dated draft.
- `bashy blog media DIR POST.md add FILE` — attach a take, scene or caption
  file; the front-matter field is chosen by kind. Composes with the
  families: `... add "$(record video --out take.mp4)"`,
  `... add "$(bashy cast record scene.cast -- make)"`.
- `bashy blog cast DIR POST.md -- CMD...` — record a terminal scene for a
  post in one step.
- `bashy blog captions DIR POST.md` — OPT-IN local transcription of the
  post's video/audio into a `.vtt` the player shows natively; named in
  `--help`, runs the local pipeline, nothing leaves the machine.
- `bashy blog record DIR POST.md [video|audio]` — OPT-IN: opens the
  person's browser for a take (the record family) and attaches it.

## Build, serve, share

- `build` uses the Hugo CLI when installed, else the bundled pure-Go
  generator (stdlib only, deterministic, no network).
- `serve` registers `public/` with the bashy console; it never starts one.
- `edit` is the bundled admin CMS at `/admin/` (Sveltia CMS, MIT, vendored
  locally — the same pinned build as examples/site).
- `publish-outpost` / `publish-pages` are the two explicit sharing acts:
  a public Outpost link from your machine, or a push to a Pages branch.

## Shaping

The journal earns the **Shapeable** badge: registered with
`bashy app add --type shaped`, declared checks in `checks/` that can fail,
per-principal activation, one-step undo. `demo/series-index/` is the demo
wish — a series index section for the home page — and
`bashy blog shape-demo` runs the whole loop: wish -> candidate -> verify
-> preview -> approve -> isolation proof -> undo.

## Anatomy

| Piece | Job |
|---|---|
| `blog.bsh` | the command: a `~~~skill` fence (what agents read) and the shell half |
| `generator/` | the pure-Go builder: `page.go` (markdown + fences), `highlight.go`, `math.go`, `diagram.go`, `cast.go` (terminal transcripts), `rss.go` |
| `sample/` | the template journal (copied by `blog new`); `sample/generator` is the same code, kept in sync by a test |
| `checks/` + `contract.bsh` | the declared verifier checks and effect cap |
| `demo/series-index/` | the shaping demo wish |
| `blog_test.go` | targeted tests: build, new, serve, smoke checks, edge cases, shaping lifecycle |

## Verified foundations & license audit

Same base as examples/site; no new fork. Permissive licenses verified
directly from upstream `LICENSE` (recorded with star counts at delivery
time; re-verify before relying on them):

| Component | Upstream | License | Pinned reference |
|---|---|---|---|
| Generator | `gohugoio/hugo` | Apache-2.0 | optional CLI; the bundled generator is our own code |
| Admin CMS | `sveltia/sveltia-cms` | MIT | tag `v0.265.1` (`9eb015aad09fa538b6ab7d032bf348ea1c0bc241`) — the same vendored `sveltia-cms.min.js` build as examples/site |

Everything else (markdown, highlighting, math, diagrams, terminal
transcripts, RSS) is code in this directory, standard library only.

## Verification

```sh
GOWORK=off go -C examples/blog test ./...   # module + script tests
bashy dag examples/blog test                # the same through the task graph
```
