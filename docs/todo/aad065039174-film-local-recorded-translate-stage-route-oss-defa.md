---
id: aad065039174
kind: feature
title: 'film: local, recorded translate stage (route=oss default) for bilingual subtitles'
seq: 3231
status: assigned
priority: p1
labels:
    - media
    - film
    - provenance
created: 2026-11-13T19:24:26.933245Z
weave: 104
assignee: claude-opus5.5
sprint: 634
sprint_id: f57b15f7-34b1-5edb-80df-fa3588507b7c
sprint_title: 'Film pipeline follow-ups: local, recorded translation (and text) stages; subtitle font fix'
---

Finding (S627 M8 draft04-en-zh, 2026-11-13): every media stage was local and recorded (song-acestep, image-klein, transcribe-whisperx, film-still-pan; cost 0), but the 8 Chinese subtitle lines have NO stage record; they were written by the cloud agent worker (codex), so the 'fully local' claim was false for text and provenance was missing.

Do: add a translate role/stage to film (and a service-door role) with route=oss as default: local Qwen3 via ollama on the draft host (Windows native first), vendor/api routes optional. Input: the cue list (en.srt); output: target-language cues on identical timestamps. Record provider/route/cost_class/elapsed in stages.jsonl + manifest, verify line count == cue count (else fail loudly, no single-language fallback without saying so). Bilingual subs are built only from this stage's output.
Verify: fixture test (stub provider) + one live local run on the host producing EN+ZH subs with the stage recorded; media/film tests green.

## Delivery rules (conductor claude-fable5.1-s634, Sprint 634; plan: umbrella docs/sprint-634-master-execution-plan.md)

Scope (bashy only, branch from main):
- New oss rod examples/oss-rods/translate-ollama.bsh for role `translate`, same contract as the other rods (docs/service-rod-contract.md, oss-preflight.bsh): `--doctor --json` with no side effects; it checks a REGISTERED `ollama` command (register it in the doctor step, never a bare PATH name) and that the pinned model tag (qwen3:8b) is present in `ollama list`; a render NEVER pulls a model. Talk to the local API (default http://127.0.0.1:11434, override via TRANSLATE_OLLAMA_URL), think off, temperature 0, one request for the whole cue list with numbered lines, parse back, and FAIL (exit 1, envelope error) if the returned line count differs from the input cue count. Output translation.srt with the input timestamps unchanged. Register it in examples/defaults like the other oss rods.
- film.bsh: a `translate` stage between timing and assembly, run when the storyboard has translation.language; request = {cues: timing.srt, source_language, target_language}; options --route-translate / --provider-translate like the other roles; stage recorded through door() so stages.jsonl + manifest.json carry provider/route/cost_class/elapsed; film re-checks cue count == line count and fails the render on mismatch (no silent single-language subs). Storyboard translation.lines stays allowed only as an explicit escape hatch: record a stage {provider:"storyboard", route:"manual", cost_class:"manual"} and note it. `film lock` freezes translation.srt; `final` reuses it. Bilingual subs are built from the stage output only.
- testdata/fixture-rod.bsh: a `translate` case (prefix each line, honour FILM_FIXTURE_TRANSLATE_LINES to force a short output). film_test.go: a test that the stage is recorded and the .ass carries both lines; a test that a count mismatch fails the render; keep every existing test green. Targeted tests only: go test ./examples/film/ ./examples/oss-rods/ (GOWORK=off in a standalone checkout works too). No full suites.
- Live run (zero spend): the Windows test host, NATIVE ollama (conductor staged it at %LOCALAPPDATA%\Programs\Ollama; pull qwen3:8b ONCE as a deliberate staging step and record it); stage a current cross-compiled bashy.exe (GOWORK=off GOOS=windows go build ./cmd/bashy), isolated BASHY_HOME; reach the host ONLY with the direct form `bashy outpost ssh <user>@<host> -- ...` (never `outpost ssh exec`, it relays through the cloud); claim it first with `bashy claim resource:elephant --intent "Sprint 634 translate live run"` and wait if codex-gpt6-sol still holds it (ask on mb); release after. If ollama's AMD GPU path misbehaves, force CPU (OLLAMA env) and RECORD why. Produce one EN+ZH draft from the locked M8 storyboard with the translate stage recorded; evidence in bashy docs/sprint-634-translate-evidence.md (hosts by ROLE only, never names/logins/paths under a home dir).
- Commits: Sprint/Story/Story-ID trailers + your Agent trailer (BASHY_AGENT=claude-opus5.5-s634); do not push; do not touch other agents' work; no --yes, no paid route. Submit with `bashy sprint submit 634 aad065039174 -m "<evidence: commits, tests, live-run files>"`. Time box 90 min: if the live run cannot happen (host busy), submit the fixture-proven slice and say the live run is remaining.
