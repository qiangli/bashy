---
id: 1ec5227e8971
kind: task
title: S190.3 examples/dag C and C++ front doors (FFmpeg, curl, git; tesseract, llama.cpp, CMake) + smoke-dag-c gate
seq: 290
status: done
priority: p0
created: 2026-09-15T10:29:54.042524Z
assignee: transom
sprint: 190
closed: 2026-09-15T11:19:05.538343Z
closed_by: transom
---

Six PR-ready dag.md graphs with the repo's own configure/build/test targets, a ~~~c / ~~~cxx smoke fence (c.ffmpeg()/c.curl()/c.git(), cxx.tesseract()/cxx.llama()/cxx.cmake() reading the checkout's own version coordinates — the checkout root as an include root where the header is self-contained — cross-checked with shell builtins) and a run target whose fence launches the repo's own built binary; scripts/dag-c-examples-smoke.sh + make smoke-dag-c; README, docs/dag.md, CLAUDE.md. Gate: make smoke-dag-c PASS on the installed binary against unchanged checkouts; smoke-dag-python + smoke-dag-typescript + smoke-dag-rust still PASS; CI=true make test.
