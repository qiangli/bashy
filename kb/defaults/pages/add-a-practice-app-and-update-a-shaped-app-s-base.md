---
id: 7a42a578-f79f-467b-b96c-8d953d3f360c
seq: 32
form: page
type: runbook
title: "Add a practice app to your console and update a shaped app's base"
description: Register the Hello World shaped app (with a tutorial video) for people to practise Software Shaping, and change a shaped app's base revision correctly — editing the registered source folder is not enough — WHEN onboarding users or members to shaping, adding a tutorial, or a change to a shaped app's files does not show up (keywords hello world, practice, tutorial, onboarding, app add, shaped, base revision, managed checkout, src, git commit).
tags:
    - shaping
    - onboarding
    - console
status: candidate
evidence: 'Live run on an owner outpost: practice app registered with an embedded tutorial; the first page edit did not show until committed in the managed checkout.'
created: "2026-11-19T04:40:40Z"
updated: "2026-11-19T04:40:40Z"
---

1. Copy `examples/shaping/hello-world` (from `bashy src fetch bashy`) to a folder and add a tutorial to `public/index.html`: keep the `<h1 id="greeting">` its check expects, embed a video lazily (`youtube-nocookie` iframe, `loading="lazy"`) plus a plain link, and list the three steps (wish button → describe the change → preview, keep or undo). To publish your own video first, see the YouTube runbook.
2. `bashy app add hello-world --type shaped --source FOLDER --label "Hello World" --tip "Practice Software Shaping"`. It appears in the console as `/hello-world/` (through the satellite: `/matrix/h/HOST/app/apps/hello-world/`, owner-only behind the host's Connect).
3. **Changing a shaped app's base later:** the console serves the committed revision of its managed checkout (`~/.bashy/src/APP`), not the folder you registered — re-adding the app keeps that checkout. Edit there and commit (`bashy git add …`, `bashy git commit -m …`; set `git config user.name/user.email` once if the commit asks for an author), or make the change as a wish so it has history and undo.
4. Members practise on their own versions: a wish creates their candidate; others keep theirs until the owner promotes a change.
