---
id: 8acb0d5f4a3a
kind: feature
title: 'S522.7 outpost-profile add --name: per-profile host name so one machine can join several satellites'
seq: 2822
status: doing
priority: p0
labels:
    - pairing
    - profiles
created: 2026-10-28T10:40:57.60732Z
assignee: genie-glm-5.3
sprint: 522
sprint_id: 3413791d-7a40-5514-9970-99b64d1d83cf
sprint_title: 'AWS cloud integration and satellite hosting: bashy aws provider (Lightsail), budget guard, hosting research, self-hosted + hosted AWS satellites'
closed_by: s522-manager
---

Operator 2026-10-28: one outpost registers with multiple satellites (work/home) and switches; do NOT move hosts. S514.5 finding 2 (umbrella origin/dev docs/sprint-514-acceptance-results.md): examples/outpost-profile/outpost-profile.bsh 'add' has no --name and always registers with the machine hostname, so a machine whose name is taken cannot add a profile. Change: add optional --name <host> (and --name=<host>) to 'add', passed through as 'outpost register --name'; default stays the hostname; 'list' shows each profile's server and host name; update the skill/help text. Tests: extend outpost_profile_test.go (fake OUTPOST_BIN records argv): --name forwarded, default unchanged, list shows name. Run only that package's tests. Pairs with cloudbox S522.6 (master accepts one assignment per satellite).

REWORK 1 (manager, 2026-10-28 12:20 PDT; live on Windows hosts elephant + puppy):
`outpost-profile add` on Windows (bashy rc.15/rc.21, no cygpath) passes `--out /c/Users/<u>/.config/matrix/profiles/<name>/agent.json`; Windows Go writes that to C:\c\Users\... , then the script reports "register reported success but wrote no file" and the profile stays (incomplete) while the host IS registered on the satellite (orphan; host name then blocked 7 days by soft-delete). Root cause: native_path() only converts when cygpath exists. Fix: on Windows without cygpath convert MSYS-style `/x/rest` -> `X:/rest` (and leave already-native paths alone); also verify the written file exists BEFORE declaring success and, on a missing file, say where outpost reported saving it. Tests in outpost_profile_test.go: native_path conversion table (/c/Users/a -> C:/Users/a, C:/x unchanged, relative unchanged) under a forced-windows switch, plus the add flow with a fake OUTPOST_BIN that writes to the path it was given. Focused test only; commit and exit.

REWORK 2 (manager 2026-10-28 13:00 PDT): START `git fetch origin s522-7-wip && git reset --hard FETCH_HEAD` (your killed run e2422a37). BUG in it: native_path prints "%s/%s" -> /c/Users/a becomes C//Users/a (no colon). Must be C:/Users/a. Also REVERT the on_windows pattern addition Linux*WSL* (WSL is Linux; it must not convert). Add the conversion table test the brief asked for (/c/Users/a -> C:/Users/a, /d -> D:/, C:/x unchanged, C:\x unchanged, //srv/s unchanged, rel/p unchanged, /home/u unchanged when not windows). Run only examples/outpost-profile tests. Commit and exit within 15 minutes.
