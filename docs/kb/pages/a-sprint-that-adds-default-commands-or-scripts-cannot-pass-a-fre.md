---
id: 01a1c838-6735-7465-b76a-8cc3817cdc5b
seq: 23
form: page
type: gotcha
title: A sprint that adds default commands or scripts cannot pass a fresh-install README acceptance until an rc is cut
description: Why a fresh-machine README acceptance needs a release first — WHEN a sprint's acceptance says 'a fresh laptop reaches X from the README' and the sprint added examples/*.bsh default commands, kb defaults or installer changes
status: candidate
source:
    tool: claude-fable5.1-s604
created: "2026-11-11T01:28:22Z"
---

A README install runs the published installer, which installs the latest promoted
release and then fetches THAT release's source pin to register default commands
(examples/defaults/bashy-defaults.bsh) and kb defaults. Nothing merged to main
after the release exists on the fresh machine: no new `examples/*.bsh` verb, no
new kb page, no installer script fix.

So an acceptance criterion of the form "a fresh Mac/Windows host reaches persona X
from the README in one agent session" is only testable after an rc that contains
the sprint's commits is promoted. Plan it as: stories close on their own criteria
(targeted tests, dry-run evidence), then the operator cuts the rc, then the
fresh-machine run happens on the fleet as the QA pass. Do not try to prove it
from a dev-box build; do not end the sprint before it.

Seen on Sprint 604 (onboard, satellite-provision, share qr): 5/5 stories
accepted, sprint held open for the rc + fresh-machine run.
