---
id: a6e1f075460e
kind: bug
title: registered 'form' command points at a deleted scratch path, so 'bashy form' fails with No such file or directory
seq: 3282
status: todo
priority: p2
created: 2026-11-21T09:08:25.951315Z
---

2026-11-21 dev box: 'bashy form' -> 'bashy: /private/tmp/claude-501/.../scratchpad/bashy-s547/examples/desk/form/form.bsh: No such file or directory'. The command was registered (register.sh: commands add form --set exec.2=$here/form.bsh) from a temporary scratch checkout, so the registration stores an absolute path that later vanished. Workaround used: run 'bashy --bashsharp bashy/examples/desk/form/form.bsh ...' from the checkout. Root cause first: confirm whether other example registrations (viewer, chart, notepad, table, ...) carry the same scratch-path exec, and decide whether registration from a temp dir should be refused or warned, or whether 'commands' should detect a missing exec target and say how to re-register.
