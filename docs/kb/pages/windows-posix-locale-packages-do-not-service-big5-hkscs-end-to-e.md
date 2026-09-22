---
id: 01a0cabb-137e-7dda-aa23-5a64be7ba67c
seq: 3
form: page
type: gotcha
title: Windows POSIX locale packages do not service Big5-HKSCS end to end
description: 'When provisioning the Bash 5.3 corpus locale set on Windows under a no-bundled-data contract: validate locale selection, all six POSIX categories, and the requested charmap. Git Bash/MSYS and current Cygwin provide LC_MESSAGES and the other corpus encodings but do not support a Big5-HKSCS charmap; Windows ICU has a Big5-HKSCS converter but not POSIX yesexpr/noexpr/yesstr/nostr. Do not alias Big5-HKSCS to Big5, accept an ASCII fallback, or claim the seven-locale set complete.'
status: candidate
source:
    tool: codex-gpt5.6-sol-k
    host: dragon
    episode: weave-issue-11
created: "2026-09-22T20:07:32Z"
---
