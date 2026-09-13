---
id: b26c70c62e16
kind: task
title: 'Output: reduction, redaction and hints validated under bashy agentic'
seq: 271
status: done
priority: p1
created: 2026-09-13T14:53:04.148371Z
weave: 16
assignee: claude-sonnet5
sprint: 166
closed: 2026-09-13T16:31:11.959798Z
---

SPRINT: #166. Validate that the SHIPPED output path does the right thing under bashy agentic - no new mechanism.
GATE: (a) reduction is on (it is already gated on BASHY_AGENTIC): a 400 KiB command output comes back under the 40 KiB ceiling with the inline marker and bashy out <handle> recovers the full bytes; --no-elide passes it through; a failing command keeps its error lines (failure is never compressed below its evidence); (b) redaction is on regardless of mode: a known vault value in stdout/stderr never appears in the envelope (test asserts strings.Contains, never prints the value); (c) hints: the advisor's failure hint and the input-story's fix notes land on stderr / in the envelope, never in stdout; (d) the same checks pass for a script and for a coreutils applet, and are skipped by design for an external (D3: no post-processing).
If (a)-(d) already hold with zero code change, this story closes on the evidence alone - that is the expected outcome.
