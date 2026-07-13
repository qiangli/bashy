---
id: 85eaee40baf6
kind: bug
title: 'weave pre-push guard (run #16): reads working-tree pins not pushed-commit; false-positives on unrelated branches'
status: open
reporter: qiangli
created: 2026-07-13T00:04:20.255367Z
---

Judge (claude-opus) revise findings on merged run #16 (pre-push guard, commit 7758b2e in 2e23f88). The .sibling-pins BUMP itself is correct and merged; these harden the added guard (follow-ups, non-blocking):

MAJOR: scripts/hooks/pre-push reads the WORKING-TREE .sibling-pins, not the pins in the commits being pushed. The prescribed flow (update-sibling-pins.sh -> commit -> push) silently PASSES if you forget the commit: working file agrees with sibling HEAD while the pushed commit still carries the stale pin CI builds. Read 'git show <local-sha>:.sibling-pins' from the refs on stdin instead.

MINOR: comparing the pin against the sibling's current HEAD (not what the branch needs) blocks pushes of unrelated branches whenever a sibling checkout moved ahead for other work; the suggested fix then bumps the pin to a possibly-unpushed sibling SHA — a false positive that trains --no-verify.

MINOR: install_hooks only inspects --local core.hooksPath, so a global/system hooksPath is silently overridden by a local setting (contradicts its own comment); both it and 'make hooks' repoint hooksPath away from .git/hooks, disabling existing local hooks there.

NIT: 'while read' drops a final line lacking a trailing newline, so a .sibling-pins without a terminating newline loses its last pin on rewrite (today's file has one; latent).
