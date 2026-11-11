---
id: 8bc5e25cfc88
kind: bug
title: M1b Windows composition fixes for media/cast/video per the Bash# tour (registered tools, ~~~powershell voice, Go-fence registration)
seq: 3221
status: todo
priority: p1
labels:
    - media
    - windows
created: 2026-11-11T07:41:46.868622Z
sprint: 603
sprint_id: c808e55d-6ce2-5525-8829-96499861c90e
sprint_title: 'Agent-assisted PDF and video: small verbs, a media skill-script and kb pages any agent can follow'
---

Windows composition fixes for the S603 examples, found by the M4 Windows UAT (2026-11-10). Root cause (recorded): the core shell works as designed; examples/media, examples/cast and examples/video were composed against the Bash# rules on Windows. Verified with current bashy main on a Windows host: ffmpeg/ffprobe on PATH by /c/ dirs and /c/ arguments work; bare names resolved from native PATH entries (powershell, bash, bashy) do not run; a Go-fence script registered by absolute /c/... path builds its overlay at \c\Users\... and resolves the wrong go.mod.

Rules to follow (bashsharp-tour 07-commands, 04-islands/go, 10-windows; sh/docs/bashpp-polyglot-fences.md):
- Tools are registered commands, never PATH programs: `media doctor` registers ffmpeg, ffprobe, agg (and pdfcraft-cli as a pinned, digest-verified download if `bashy commands add` supports it, else its verified cache) via `bashy commands add NAME --set exec.0=<path>`; media/video call only those names.
- Windows narration is a `~~~powershell` fence (pinned PowerShell 7, System.Speech), not `powershell` from PATH. macOS `say`/Linux `espeak-ng` stay registered commands.
- media runs video.sh through bashy (a registered `video` command or the running bashy), never `bash`.
- Go-fence scripts (examples/cast/cast.bsh, examples/record/record.bsh) are registered/run the way the tour runs islands (from their own directory next to their go.mod, relative name) or with a native path; `doctor` makes sure `go` is available (bashy go provisioner).
- Hash downloads from stdin; unzip without bashy tar (registered tool or a ~~~powershell Expand-Archive).
- kb pages: add the registration step (`bashy-defaults apply`, or `bashy commands add` with effects), `--voice` as a draft aid, a direct-download fallback when winget's source fails; windows-remote-ops page: `outpost ssh HOST -- EXE`, and `outpost scp --safe` is uploads-only.
Acceptance: the M4 Windows run (PDF 2 pages; 60 s video with one cast terminal scene and captions; OS voice draft) succeeds on the Windows host with current bashy and NO manual workarounds; macOS tests (examples/media/test.sh, examples/video/test.sh with GOWORK=off) stay green.
