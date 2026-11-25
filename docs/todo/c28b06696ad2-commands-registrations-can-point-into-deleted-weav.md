---
id: c28b06696ad2
kind: bug
title: 'commands: registrations can point into deleted weave workspaces (stale notepad on dev box)'
seq: 3314
status: todo
priority: p3
labels:
    - desk
created: 2026-11-25T10:47:19.289735Z
---

Symptom (2026-11-25, dev box): `bashy notepad new …` exits 127, 'No such file or directory' for $HOME/.bashy/weave/bashy-6497d06f/workspaces/issue-349/examples/desk/notepad/notepad.bsh. The registered verb enum is serve,new,open,get,put,list,share,close, while the skill guide (and current source) say add/show/set….

Root cause (inspected): the host's notepad registration was made from a weave workspace (issue-349) by an older build with the old verb set; the workspace was later deleted. The current source agrees with itself: examples/desk/notepad/notepad.bsh and kit.ToolVerbs("notepad") both use add/show/set/…. The registration is stale; the guide is not wrong.

Classification: environment (stale host registration) exposing a product gap: nothing notices when a registration's exec path disappears. Weave cleanup and 'bashy commands' do not check it, and register.sh from a workspace pins a path that will be deleted.

Fix: (1) host: re-run examples/desk/notepad/register.sh from the umbrella checkout; (2) product: `bashy commands verify` (or list) flags registrations whose exec path is missing, and weave cleanup warns when it deletes a path a registration uses.

Verify: `bashy notepad add --title T -f FILE` works; a test registration pointing at a removed directory is reported.
