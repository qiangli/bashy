# examples/film — a music video from one storyboard

`film.bsh` (Sprint 627 M7; plan: the umbrella's
`docs/sprint-627-master-execution-plan.md`) chains the service doors into a
60–90 s music video. Drafts run on open-source rods as often as wanted; a
lock freezes what the operator liked; the final re-renders only the
generative stages on a commercial route. Registered by `bashy-defaults` as
`bashy film`, or run `bashy examples/film/film.bsh VERB`.

| verb | does |
|---|---|
| `film doctor` | checks ffmpeg, ffprobe, jq on PATH and `bashy ffmpeg/ffprobe/agg` (registered by `media doctor`), the door and media |
| `film init DIR [--title T]` | writes `DIR/storyboard.json` (`bashy-film-storyboard-v1`) to edit |
| `film draft DIR` | `DIR/drafts/NN/film.mp4` on route `oss` (default) |
| `film lock DIR [--draft N] [--timing FILE]` | `DIR/lock/`: storyboard, timing, shot bounds, duration, portraits, with `lock.json` sha256 per file |
| `film final DIR --route browser\|api [--yes] [--dry-run]` | `DIR/finals/NN/film.mp4` from the lock |
| `film status DIR` | one line per render: kind, id, route, outcome, elapsed, cost classes, QA verdict |

Render options: `--provider-{song,image,animate,transcribe} NAME`,
`--route-{song,image,animate,transcribe} ROUTE`, `--qa-provider NAME`
(default: the selector's `video.understand` choice, agy), `--no-qa`,
`--json` (print the manifest instead of its path).

## Pipeline

Each generative stage is one call to `examples/service-door/service-door.bsh`
(the body behind `bashy song`, `bashy image`, …; contract
`docs/service-rod-contract.md`) with a `bashy-svc-request-v1` file:

1. `song` — `prompt`, `lyrics`, `duration_s`, `language`, `seed`.
2. `image.generate` — one portrait per character from its bible (one
   consistent set; the final passes the locked portrait as
   `reference_images`).
3. `transcribe` (draft only) — the song's lyric timing as SRT. The final
   reuses the locked timing.
4. Shot bounds — a shot with `line: N` starts at lyric line N's cue; others
   split the remaining time evenly. The final reuses the locked bounds.
5. `animate` — one clip per shot: the character's portrait plus that shot's
   slice of the song (lip sync). A draft with no animate rod renders a still
   portrait with a slow pan, labelled `still image with a slow pan` in the
   manifest's `degraded`. A final never degrades.
6. Assembly — `media cut` on the shot beats, the song muxed under it, `media
   cards` for the title and end cards (the end card carries the
   AI-generated disclaimer), `media subs` burning bilingual subtitles
   (timing + `translation.lines`, line for line) and muxing them as a soft
   track.
7. QA — `film.mp4` and a review question through the `video.understand`
   door (`bashy video`, agy). The reply's `{"pass":…,"issues":[…]}` becomes
   `qa.json` (`pass`, `fail`, `unparsed` or `unavailable`). QA records a
   verdict; it does not fail the render.

Every door envelope (provider, route, `cost_class`, `elapsed`) is kept in
the render's `manifest.json` (`bashy-film-render-v1`), with the storyboard's
sha256 and, for a final, the lock's.

## Spend and consent

`final` refuses `--route oss`, needs a lock, and checks every locked file's
sha256 before any stage runs. Assembly tools are checked first too, so a
final cannot spend and then fail at assembly. Commercial rods declare
`spend`; without `--yes` the door names them with "rerun with --yes", no rod
runs, and film exits 126. `--dry-run` names the rod each generative stage
would use and writes `finals/dry-run/manifest.json`. Passing `--yes` is the
operator's decision.

## Test

```bash
GOWORK=off go -C examples/film test ./...
```

Fixture rods only (`testdata/fixture-rod.bsh` serves every role with small
ffmpeg artifacts; no model, download, network or spend). The tests run in a
scratch command ring under a freshly built bashy with the real door and
selector, and cover:

- draft → lock → final (stage order, the locked timing and portrait reused,
  bilingual subtitles, stream layout, duration);
- the `--yes` refusal and `--dry-run`;
- lock tampering and storyboard drift;
- the still-pan fallback, and the final refusing to degrade;
- QA verdicts;
- storyboard validation.

They need the host's ffmpeg, ffprobe and agg, as `examples/media/test.sh`
does, and skip without them. Real oss drafts run on a claimed test host. The
commercial final is an operator-approved run.
