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

## Delivery rules (conductor claude-fable5.1-s634, Sprint 634; plan: umbrella docs/sprint-634-master-execution-plan.md)

Starts AFTER story aad065039174 (translate stage) is merged: reuse its ollama rod, do not write a second ollama client.
- Rod: extend examples/oss-rods/translate-ollama.bsh (or its sibling, keep ONE ollama client) to also serve role `text` (provides.N=text in examples/defaults): request {brief, kind: lyrics|shots|prompts, language, count/duration hints, storyboard (optional, for shots/prompts)} -> `text.json` with the drafted lines/objects; same doctor, same pinned qwen3 tag, think off, temperature from the request (default 0.7 for drafting), never pulls.
- film.bsh: one new verb `film write DIR [--route oss] [--provider-text NAME] [--kind lyrics|shots|prompts|all]` that reads DIR/brief.md (title, theme, characters, mood; create a short example in the README) and writes/updates storyboard.json fields (lyrics, shots[].description, shots[].prompt, characters[].prompt) WITHOUT touching fields the brief does not cover; every call records a stage through door() into DIR/text/NN/stages.jsonl + manifest.json (provider, route, cost_class, elapsed, which fields were written). A storyboard edited by hand afterwards stays valid; `film draft` is unchanged except that manifest.json also copies the latest text provenance when DIR/text exists.
- Tests: fixture rod `text` case (deterministic lines from the request); film_test.go: `write` records the stage and fills the fields; a second `write --kind shots` leaves lyrics untouched. Targeted only: go test ./examples/film/ ./examples/oss-rods/.
- Live run (zero spend): the Windows test host, NATIVE ollama (already staged; model qwen3 tag already pulled by the translate story); claim resource:elephant first (`bashy claim`), release after; reach it only with `bashy outpost ssh <user>@<host> -- ...` (never `outpost ssh exec`); one `film write` on a small brief, then one `film draft --route oss` on the result; evidence in bashy docs/sprint-634-text-evidence.md (hosts by ROLE only).
- Commits: Sprint/Story/Story-ID trailers + your Agent trailer; do not push; no --yes, no paid route; do not touch other agents' work. Submit with `bashy sprint submit 634 759ea0f92839 -m "<evidence>"`. Time box 90 min.
