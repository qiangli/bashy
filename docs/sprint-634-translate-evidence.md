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

## Live run: NOT done (remaining)

At 2026-11-13 ~20:45Z `bashy claim resource:elephant` succeeded (the Windows
test host was free). The host did not answer on the LAN:

- ping: 100% loss.
- `bashy outpost ssh` (the direct form): `lan-direct dial …:2222: i/o
  timeout` on IPv4 and IPv6.
- It was still unreachable when retried about 7 minutes later.

The claim was released and nothing ran on the host.

Remaining, zero spend:

1. Claim the Windows test host.
2. Run `film doctor` to register native ollama, then
   `bashy translate-ollama --doctor --json` should report available, with
   `qwen3:8b` staged by the conductor.
3. Using a current `GOWORK=off GOOS=windows go build ./cmd/bashy` and an
   isolated `BASHY_HOME`, produce one EN+ZH draft from the locked M8
   storyboard (`translation: {language: "zh"}`, lines removed). Record the
   `translate` stage (provider `translate-ollama`, route oss, cost_class
   oss, elapsed) from `stages.jsonl` / `manifest.json`.
4. Append the results here. If ollama's AMD GPU path misbehaves, force CPU
   and record why.
