# Sprint 634: local, recorded translate stage — evidence

Story `aad065039174` (Sprint 634, story #3231). Plan of record: the umbrella's
`docs/sprint-634-master-execution-plan.md`, step 3.

## Finding this closes

S627 M8 draft04-en-zh: every media stage was local and recorded, but the
eight Chinese subtitle lines had no stage record. A cloud agent worker wrote
them, so "fully local" was false for text and provenance was missing.

## What changed (commit `54f796c1`)

- `examples/oss-rods/translate-ollama.bsh`: an oss rod for role `translate`.
  It sends the cue list (SRT) to a local Qwen3 (`qwen3:8b`) through ollama's
  HTTP API (default `http://127.0.0.1:11434`, `TRANSLATE_OLLAMA_URL`). It
  makes one chat request with numbered lines, `think: false` and
  temperature 0. It writes `translation.srt` with the input timestamps
  verbatim. When the reply's numbered line count differs from the cue
  count, the rod exits 1, the envelope is `error`, and no file is written.
  - `--doctor --json` has no side effects. It runs `list` through a
    registered `ollama` command (`TRANSLATE_OLLAMA_COMMAND`), never a bare
    PATH name, and checks the pinned tag.
  - A render checks `/api/tags` and never pulls. A missing model fails with
    the staging line.
- `examples/film/film.bsh`: a `translate` stage between timing and assembly
  when the storyboard has `translation.language`.
  - The request is `{cues: timing.srt, source_language, target_language}`,
    and `--route-translate` / `--provider-translate` work as for the other
    roles.
  - The stage is recorded through `door()`, so `stages.jsonl` and
    `manifest.json` carry provider, route, cost_class and elapsed.
  - film checks the line count against the cue count again. A mismatch, or
    no usable translate rod, fails the render. It never falls back to
    single-language subtitles.
  - Bilingual subtitles are built only from `translation.srt`.
  - `translation.lines` still works as an explicit escape hatch. It is
    recorded as `{provider: storyboard, route: manual, cost_class: manual}`
    and noted on stderr.
  - `film lock` freezes `translation.srt` on the locked timing and refuses
    a cue-count mismatch. `film final` reuses it, and refuses a lock made
    before this stage existed.
  - `film doctor` registers the host's ollama where bashy does not ship its
    own `ollama` verb.
- `examples/defaults`: registers `translate-ollama` (route oss, `has=curl
  has=jq`) and the `translate` door verb. `docs/service-rod-contract.md`
  lists the `translate` role.

## Tests (dev box, fixture/stub providers only, no model)

| Command | Result |
|---|---|
| `go test ./examples/film/` | ok (156.7 s): the 9 existing tests plus `TestDraftTranslateStageRecorded`, `TestDraftTranslateMismatchFails`, `TestDraftTranslationLinesManual` |
| `go test ./examples/oss-rods/` (with `OSS_AVAIL_KB_OVERRIDE`) | ok: `TestTranslateOllamaRod` (httptest ollama stub: success, short reply → exit 1, unstaged model → refused, no pull), `TestTranslateOllamaDoctor` (bare PATH ollama does not count; registered one does; missing tag reported) |
| `go test ./examples/defaults/` | ok |

On the dev box, the oss-rods image cases need `OSS_AVAIL_KB_OVERRIDE`. The
box has 32 GiB free and the image rods' preflight wants 35 GiB. That is
unrelated to this change.

## Live run (Windows test host, 2026-11-13, conductor claude-fable5.1-s634)

Done by the conductor after the worker found the host unreachable (a
transient; it answered again minutes later). Zero spend; nothing paid or
`--yes`; no GPU job: ollama ran the 8B model on the host CPU/iGPU path by
itself, which is not a diffusion VAE decode (the TDR rule).

Venue staged, all native (no WSL):
- ollama v0.40.16 from the official standalone `ollama-windows-amd64.zip`
  (sha256 verified against the release's `sha256sum.txt`), extracted under
  the per-user Programs folder, `ollama serve` on 127.0.0.1:11434. The
  Inno Setup installer refuses to run from a non-interactive ssh session and
  winget's source is broken on that host, so the zip is the route.
- `qwen3:8b` (Q4_K_M, 5.2 GB) pulled once as a staging step.
- A sprint `bashy.exe` cross-built WITH the umbrella `go.work` (a
  `GOWORK=off` build lacks the `route` command path the defaults need),
  placed first on PATH; an isolated `BASHY_HOME` / `BASHY_COMMANDS_DIR`;
  the sprint `examples/` tree extracted over the fetched source pin;
  `bashy-defaults apply --force`; the host binaries registered as
  `ollama-host` and `curl-host`.

Run: the translate door with the 8 locked M8 English cues (`en.srt` from
the S627 draft04-en-zh review folder), request
`{cues, source_language: en, target_language: zh, title}`:

```
bashy translate --route oss --json --out OUT translate.request.json
```

Envelope (exit 0): provider `translate-ollama`, route `oss`, cost_class
`oss`, outcome `success`, elapsed 6 s, text "oss translation via qwen3:8b
(ollama): 8 cue(s) en -> zh", one output `translation.srt`. The model
answered 8 numbered lines (eval 84 tokens, 5.7 s total, chat.response.json
kept); `translation.srt` carries the 8 input timestamps unchanged. The cues
were then burned with `media subs en.srt translation.srt` on the dev box:
the spot frame at cue 3 shows "Hold the light and sing it low" in IBM Plex
Mono over the centred Chinese line (the Sprint 634 font fix). Files on the
dev box: the umbrella's `.agents/s634-review/translate-live/` (envelope,
request, chat request/response, reply, translation.srt, subs.ass, burned
clip, spot frames).

Gaps the live run exposed, each root-caused and fixed where the cause
lives (commits on bashy main, Story-ID aad065039174):
1. `bashy commands add ollama` is refused on every OS (bashy ships that
   verb): film doctor registers `ollama-host`, the rod's default.
2. `has=jq` in a rod's requires probes with `command -v`, which cannot see
   bashy's in-process applets; native Windows has no jq binary. The rod
   requires only curl; the shared gap is todo `f9e0a8bd`.
3. On Windows the door canonicalised the request and `--out` paths with
   `cd && pwd` into `/c/...`, which the Go selector and native curl cannot
   open ("no provider declares translate", then an empty chat response).
   The door now hands native paths to native consumers (merged with the
   text story's equivalent fix).
4. `oss_request_get` trusted a `python3` that `command -v` finds but cannot
   run, and its jq fallback used `--arg`, which bashy's jq lacks: it reads
   through jq with environment bindings.
5. A bare `curl` found on PATH does not run under bashy on Windows and the
   name is bashy's own verb: the rod uses a registered `curl-host` when one
   exists (film doctor registers the system curl.exe), else `curl`.
6. On Windows `bashy-defaults apply` registers whichever `bashy` is first on
   PATH; a stale outpost binary there masquerades as a product bug.
