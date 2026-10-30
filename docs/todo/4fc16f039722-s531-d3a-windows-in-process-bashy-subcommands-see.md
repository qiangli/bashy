---
id: 4fc16f039722
kind: bug
title: 'S531.D3a Windows: in-process bashy subcommands see the shell''s POSIX USERPROFILE/HOME'
seq: 2834
status: done
priority: p0
labels:
    - windows
    - genie
created: 2026-10-29T16:29:30.916452Z
weave: 338
assignee: claude-sonnet5.5
sprint: 531
sprint_id: f7bc2570-ca8e-59b0-8163-c3c51ba23610
sprint_title: 'Genie core harness: drive vendor agent CLIs as tools'
closed: 2026-10-29T16:53:03.936569Z
closed_by: s531-manager
---

Found in the S531 D3 live run on a native Windows test host (operator principle 2026-10-29: same behavior on every OS, zero setup; a divergence is a bug).

Repro (native Windows, session shell = bashy, e.g. under `outpost ssh`): the shell spells HOME and USERPROFILE as /c/Users/<u> (shellStartupEnv, by design for scripts). `bashy genie -m <api-model> prompt "..."` then resolves os.UserHomeDir() from that POSIX spelling, i.e. C:\c\Users\<u>, so the fleet model store (~/.config/bashy/models) and secrets.map are not found and genie falls through to the local Ollama path ("pulling <model> (first use)" / engines build error). The bar cache lands under \c\Users\<u>\.bashy, and a stray C:\c\Users\<u> directory gets created. With USERPROFILE='C:\Users\<u>' HOME='C:\Users\<u>' prefixed it works (external model, correct answer).
Same failure with the session shell being an older bundled bashy or the current build (bashy main + yoke/ycode main), so this is not only a stale-binary issue. sh/interp nativeExecEnvMountsMode already converts USERPROFILE/TMP/TEMP for exec'd NATIVE children; the gap is the in-process path: bashy subcommands that the shell runs in-process (or a bashy process started with the shell's env) read the POSIX spelling directly.

Fix (minimal, root cause): the Go side of bashy must resolve the host home/temp from native spellings on Windows, whatever path got there — e.g. normalize USERPROFILE/HOME/TMP/TEMP (/x/... -> X:\...) for the process's own os.UserHomeDir/os.TempDir view at bashy entry and at the in-process subcommand seam, without changing the shell-visible $HOME spelling. Pick the narrowest seam that covers both cases; do not change Linux/macOS behavior.
Acceptance: unit red/green (windows build tags or injectable env): a bashy entry with USERPROFILE=/c/Users/x resolves home C:\Users\x; shell $HOME stays /c/Users/x; existing invocation/history tests stay green. Live check (manager): `bashy genie -m <door model> prompt` on native Windows under outpost ssh with no env prefix answers correctly. Generic commit wording; no hostnames.
