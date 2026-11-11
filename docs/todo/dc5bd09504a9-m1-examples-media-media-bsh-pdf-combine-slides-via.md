---
id: dc5bd09504a9
kind: feature
title: 'M1 examples/media/media.bsh: pdf/combine/slides via pdfcraft-cli, narrate, build — parity on macOS/Windows/Linux'
seq: 3213
status: todo
priority: p1
labels:
    - media
    - examples
created: 2026-11-11T02:40:04.652565Z
sprint: 603
sprint_id: c808e55d-6ce2-5525-8829-96499861c90e
sprint_title: 'Agent-assisted PDF and video: small verbs, a media skill-script and kb pages any agent can follow'
---

Spec: dhnt-docs docs/sprint-603-master-execution-plan.md (M1). Supersedes dhnt-docs story 3358d726c018 (B2).

GOAL: `examples/media/media.bsh`, a Bash# skill-script (one file = command + `~~~skill as guide`, like examples/record/record.bsh), that packages the proven macOS PDF/video workflow so it gives the SAME result on macOS, Windows and Linux. Parity is the requirement: no OS-only documented path; `doctor` fails with the exact install line instead of degrading.

Verbs (each works alone, plain files in/out, prints only output paths):
- `media doctor` — download a pinned `pdfcraft-cli` release (MIT OR Apache-2.0, github.com/storytold/pdfcraft) per OS/arch, SHA-256 verified, into the bashy cache; check ffmpeg + ffprobe (require an H.264 encoder and the `subtitles` filter), agg, a voice tool; print the per-OS install command (brew / winget / apt) for anything missing. Never bundle or download ffmpeg or agg (GPL).
- `media pdf IMG... -o OUT.pdf [--title T --author A]` — images to PDF via `pdfcraft-cli run` (`doc_create {"from":"images","paths":[...]}` then `doc_save`), then `pdfcraft-cli edit --title/--author`.
- `media combine A.pdf B.pdf... -o OUT.pdf` — `pdfcraft-cli combine`.
- `media slides IN.pdf DIR` — `pdfcraft-cli render` each page to DIR/NN.png (for the video).
- `media narrate PROJECT --own | --voice V` — `--own` (the documented path): for each line of PROJECT/narration.txt, take the member's own recording (made with `record audio --script`) and fit it to its segment; `--voice` uses the OS voice as a draft aid only (macOS `say`, Windows `System.Speech` via PowerShell, Linux `espeak-ng`). Port the fitting/loudness method of dhnt-docs docs/sprint-523/demo-video/narrate.sh (no hardcoded paths).
- `media build PROJECT` — calls `examples/video/video.sh build`; `camera` segments read files made by `record video`; `step` segments record with `cast record` (yoke story M2) when present, else asciinema; agg always gets the pinned OFL monospace font shipped under examples/media/fonts/.
- Shared projects: accept CRLF or LF in storyboard.txt / narration.txt and write LF; all project paths relative; sample ships `.gitattributes`.
- A small generic sample project under examples/media/sample (2 slide PNGs, 1 narration file, storyboard) — generic owner/member wording, no real names.
- No Chrome anywhere in media.

FIRST CHECKS (record results in the commit message): pdfcraft-cli release asset names + checksums for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64, windows/amd64; agg availability on winget/Scoop; that bashy runs what typed.sh/video.sh use (process substitution, read -t, read -N, EPOCHREALTIME).

TEST: `examples/video/test.sh` stays green; add `examples/media/test.sh`: pdf from two PNGs -> 2 pages (`pdfcraft-cli info`), slides round-trip, and the integration: sample -> narrate --voice (fixed short text) -> build -> ffprobe shows the duration and an audio stream. Dev box only, no podman/VM.

Agent plan: raced — several vendors each build one; the manager compares and keeps the best.
