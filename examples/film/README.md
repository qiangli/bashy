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

## Lip sync on a local GPU (Sprint 635)

`dag.md` is a literate task file for real lip-synced takes with the
open-source `animate-echomimic` rod (EchoMimicV3 Flash on a local NVIDIA GPU,
Windows native first). Run it with `bashy dag -f examples/film/dag.md TARGET
PROJECT=DIR [SHOTS="3 5"]` from anywhere; bodies call only registered commands
(`bashy animate-echomimic`, `bashy film-lipsync`, `bashy film`).

| target | does |
|---|---|
| `stage-lipsync` | `animate-echomimic --stage`: pinned venv, packages, source, patched copy, weights, ffmpeg; idempotent |
| `doctor-lipsync` | `animate-echomimic --doctor --json` (read-only; the default target) |
| `proof-lipsync` | one 5 s take from the first close-up shot's locked still and sung segment: `PROJECT/takes/NN.mp4` + `takes.json` |
| `takes-lipsync` | every close-up shot (`kind: close`) of the locked storyboard whose still exists, full segments, same `takes.json` |
| `draft-lipsync` | `film draft PROJECT --takes PROJECT/takes --locked --route oss` |
| `review-lipsync` | contact sheet, mouth-region grid and mouth motion metric per take into `PROJECT/takes/review/` |

`film-lipsync` (`lipsync.bsh`, registered by `bashy-defaults`) is the body
behind the take and review targets. `takes` needs `film lock` first: it reads
`lock/storyboard.json`, `lock/shots.tsv`, `lock/portraits/` and the locked
draft's song, sends each shot through the animate door with `--provider
animate-echomimic` (`--provider` overrides) and `"model":"echomimicv3-flash-pro"`,
and records `takes.json` (`bashy-film-takes-v1`: file, provider, model,
revision, elapsed_s, sha256, still, seconds) — the manifest `film draft
--takes` and `animate-prerendered` verify. `review` writes `NN-sheet.png`,
`NN-mouth.png` and `review.json` with the tblend=difference + signalstats
YAVG mean/max over the mouth region (the Sprint 635 evidence's measure).

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
- `film-lipsync takes --proof|--shots` → `takes.json` with sha256, `review`,
  and `film draft --takes` assembling the take (fixture rod as the provider).

They need the host's ffmpeg, ffprobe and agg, as `examples/media/test.sh`
does, and skip without them. Real oss drafts run on a claimed test host. The
commercial final is an operator-approved run.

### Draft the text locally

Start with `film init my-video`, then write `my-video/brief.md`, for example:

```markdown
Title: Morning by the sea
Theme: Greeting a new day beside the sea.
Characters: lead, an original adult singer in a blue coat.
Mood: Warm and hopeful, quiet sunrise.
Lyrics: Two short English lines.
Shots: Describe the two existing shots: medium, then wide.
Prompts: Sunrise image prompts for both shots and the lead portrait.
```

Run `film write my-video --route oss --provider-text translate-ollama`.
The shared Ollama rod uses the already staged `qwen3:8b` tag, thinking off,
with drafting temperature 0.7 (a direct text request may override it).
It never pulls a model. The rod's existing doctor and `TRANSLATE_OLLAMA_*`
settings apply to both translation and drafting.

`--kind lyrics|shots|prompts|all` selects the prose to draft (default `all`).
The brief is passed intact with the storyboard, language and count/duration
hints. The provider returns only objects covered by the brief. `write` applies
lyrics, shot descriptions, shot prompts and character prompts by existing
index/id; omitted objects and all other storyboard fields are preserved.
Use `--kind` to enforce a narrow edit independently of the model, for example
`film write my-video --kind shots` leaves lyrics and prompts untouched.
Initialize the storyboard's characters, shot layout and production settings
before writing; this command does not create or renumber them.

Each call saves its brief, requests, provider outputs, before/after storyboards,
`stages.jsonl` and `manifest.json` under `text/NN/`. Each successful stage lists
`fields_written` along with provider, route, cost class and elapsed time.
Invalid output fails before applying that stage; earlier successful stages
remain recorded. Human edits afterwards are valid: `film draft my-video
--route oss` copies the latest text manifest into `text_provenance` without
asserting that the current storyboard is still byte-identical to model output.
The provenance includes the original storyboard hash for comparison.
