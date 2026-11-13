---
id: 759ea0f92839
kind: feature
title: 'film: optional local text-drafting stage (lyrics / storyboard prose) with recorded provider'
seq: 3232
status: todo
priority: p2
labels:
    - media
    - film
    - provenance
created: 2026-11-13T19:24:27.092262Z
sprint: 634
sprint_id: f57b15f7-34b1-5edb-80df-fa3588507b7c
sprint_title: 'Film pipeline follow-ups: local, recorded translation (and text) stages; subtitle font fix'
---

So a draft can be fully local: lyrics, shot descriptions and image prompts drafted by a local LLM (route=oss, e.g. Qwen3 via ollama) instead of the orchestrating cloud agent; agents may still edit, but the stage records which provider produced the text. Record in stages.jsonl/manifest like media stages. Verify: fixture test + one live local draft.
