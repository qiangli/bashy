---
id: b98882b060f2
kind: bug
title: bashy app add writes ~/.config/bashy/apps even with an isolated BASHY_HOME (sandboxes clobber real tiles)
seq: 2966
status: todo
priority: p1
labels:
    - app
created: 2026-10-30T20:58:43.021174Z
---

Found during Sprint 537 (2026-10-30): `bashy app add NAME --type shaped --source DIR` (and plain app add) writes the tile record to ~/.config/bashy/apps/NAME.yaml even when BASHY_HOME points at an isolated directory. Sandboxed tests and agent repros therefore overwrite the operator's real launcher registrations (S537 S1c worker overwrote apps/{notepad,chart,form,viewer,whiteboard,brainstorm}.yaml at 13:49-13:55 PDT; the manager's isolated fork proof at ~13:20 did the same for notepad).
Expected: the app registry follows the same state root as the rest of bashy ($BASHY_HOME, else ~/.bashy or the documented XDG location) so one variable isolates everything; per feedback "bashy/ycode state under $BASHY_HOME/~/.bashy". Red/green: app add with BASHY_HOME=tmp leaves ~/.config untouched.
