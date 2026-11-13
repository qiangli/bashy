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
assignee: claude-opus5.5
sprint: 634
sprint_id: f57b15f7-34b1-5edb-80df-fa3588507b7c
sprint_title: 'Film pipeline follow-ups: local, recorded translation (and text) stages; subtitle font fix'
---

Finding (S627 M8 draft04-en-zh, 2026-11-13): every media stage was local and recorded (song-acestep, image-klein, transcribe-whisperx, film-still-pan; cost 0), but the 8 Chinese subtitle lines have NO stage record; they were written by the cloud agent worker (codex), so the 'fully local' claim was false for text and provenance was missing.
Do: add a translate role/stage to film (and a service-door role) with route=oss as default: local Qwen3 via ollama on the draft host (Windows native first), vendor/api routes optional. Input: the cue list (en.srt); output: target-language cues on identical timestamps. Record provider/route/cost_class/elapsed in stages.jsonl + manifest, verify line count == cue count (else fail loudly, no single-language fallback without saying so). Bilingual subs are built only from this stage's output.
Verify: fixture test (stub provider) + one live local run on the host producing EN+ZH subs with the stage recorded; media/film tests green.
