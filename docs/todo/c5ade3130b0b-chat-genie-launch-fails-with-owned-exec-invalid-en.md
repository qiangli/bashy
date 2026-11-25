---
id: c5ade3130b0b
kind: bug
title: 'chat: genie launch fails with ''owned exec: invalid environment entry'' on a large instruction'
seq: 3316
status: todo
priority: p3
labels:
    - agents
    - genie
created: 2026-11-25T15:54:42.066155Z
---

Seen 2026-11-25 (umbrella Sprint 712 vendor review). `bashy chat --agent genie-glm-5.3 --read-only -m "<~3 KB brief>" --file <two docs, ~60 KB>` exited 1 before the model ran:
  owned exec: invalid environment entry
  run: bashy dag --quiet --file dag.md chat (cwd: the genie bars cache), exit 1, 217 ms
The same agent with a one-word instruction works. The same instruction and files work for codex, agy and muse through `bashy chat`. So the size or content of the instruction, apparently passed to the genie dag through the environment, is the suspect. Not proven.
Workaround: a short instruction that names the files to read, run with --cwd at the repo.
Next step: root cause (what env entry is invalid: size, NUL, '=' in name?), then a fix and a test with a large instruction.
Side note from the same review: muse and agy write their raw event streams (JSONL) with --full, so the answer text has to be extracted; worth a check whether --full should return the final text.
