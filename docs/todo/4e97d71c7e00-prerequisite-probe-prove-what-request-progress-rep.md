---
id: 4e97d71c7e00
kind: task
title: 'PREREQUISITE PROBE: prove what request-progress reporting already reaches the operator, and where it stops'
seq: 224
status: done
priority: p0
created: 2026-09-06T00:38:00.281928Z
assignee: codex-gpt5.6-sol
sprint: 126
closed: 2026-09-06T03:44:54.748374Z
---

BLOCKS #216 (M3). M3 says "the specific symptoms are operator OBSERVATIONS and are not
yet recorded here" — this story records them, with a red browser case per symptom.
No fix lands under M3 until this closes.

THE REQUIREMENT (operator, verbatim in substance): when a user or agent sends a
message, there must be CHEAP ways — little to no token cost, minimal network — to
report progress to the sender. For humans, a visual indicator with cumulative
results (lines/bytes rising, or percent remaining). Roughly: sent -> received ->
processing -> detailed progress -> done/error. Nobody should stare at a screen for
minutes unsure whether the message was even accepted. Agents need the same signal to
monitor and check a delegated request.

MEASURED STATE, 2026-09-06 (read from source, not assumed). MOST OF THIS IS BUILT:

  pkg/meet/live.go is exactly this feature and it already ships. Two channels, one
  truth: transcript.jsonl is the RECORD (one event per completed turn), live.jsonl is
  the VIEW (line-granular tee of the agent stdout, ephemeral, safe to lose). Kinds:
  speaking (took the floor) / line (one line) / spoke (finished, carries Status).

  LiveEvent ALREADY carries Lines, Bytes and ElapsedMS — the exact cumulative
  counters the requirement asks for.

  pkg/meet/relay_dm.go dmProgressSampler is already CHEAP BY CONSTRUCTION: frames are
  emitted on FIBONACCI line numbers (1,1,2,3,5,8,13,21,...), so report density falls
  as the answer grows and total frames are logarithmic in output size; each frame
  carries a length-capped snippet, plus a heartbeat when a turn goes quiet. Zero
  extra model tokens: it is a tee of bytes the agent was already writing.

  The shipped SPA consumes it: pkg/meet/artifact/assets/index-*.js references
  /observe, /observe-dm, elapsed_ms, speaking, spoke and line.

SO THE GAPS ARE NARROW, AND THESE ARE THE THINGS TO PROBE:

  G1  ROOMS DO NOT GET IT. live.go says the counters are "populated by the DM
      observer bounded progress projection ... and therefore remain absent from
      ordinary meeting live events." So a DM shows progress and a ROOM does not —
      and rooms are where sprint work is discussed. PROBE: post to an agent in a
      room and in a DM, capture both /observe frames, diff them.

  G2  A JOB CAN BE CANCELLED BUT NOT ASKED ABOUT. The long verbs (ask, round, poll,
      address, converge) return 202 + JobRef{job,room}. pkg/meet/recall.go already
      tracks per-job committed/recalled/finished server-side. The ONLY verb on that
      id is recall (cancel): the route list has no job status read path. So a sender
      holding a job id cannot ask "is it running" — which is precisely the agent half
      of the requirement. PROBE: confirm no GET resolves a JobRef.

  G3  DOES IT ACTUALLY RENDER? Everything above is source, not behaviour. The M3
      symptom is that output does not reach the screen. PROBE IN A REAL BROWSER via
      the existing gate (go test ./pkg/webconsole -tags verifydom): send to an agent
      from the Meet tab and from the Chat tab, and record for each — does anything
      appear at send time, does a progress indicator appear while it works, does the
      final answer render, and what happens on error. One red case per symptom.

  G4  DELIVERY vs EXECUTION are different axes and only one is modelled. Sprint 126
      binds the DELIVERY vocabulary (accepted, queued, delivered, read, failed,
      unverified) and it ENDS AT read. What happens after read — the agent working —
      is the axis this requirement is about. PROBE: confirm the two are separable in
      the UI, because collapsing them would let "delivered" read as "being worked on".

HONEST LIMIT — DO NOT SHIP A PERCENTAGE. The requirement asks for "percentage /
percent remaining". An agent turn has NO KNOWN TOTAL: nothing knows how many lines an
answer will be, so any percentage would be fabricated. That is the exact class of
confidently-wrong signal this codebase refuses everywhere else (see the harness exit
codes finding: all three exited 0 while failing). The honest substitutes already
exist and should be used instead: elapsed time, cumulative lines/bytes, a heartbeat
proving liveness, and a last-line snippet showing WHAT it is doing. A rising counter
with a heartbeat answers "is it alive and moving" — which is the real question behind
the request — without inventing an end it cannot see.

ALSO NOT A PERCENTAGE, BUT CLOSE: where a total IS known the percentage is real and
should be shown (weave run points, stories closed of N, gate steps). Do not
generalize that to a single turn.

DELIVERABLE OF THIS STORY: no fix. A written symptom list (page, tab, expected,
actual) plus one RED verifydom case per symptom, committed red, and G1/G2/G4
confirmed or refuted against the code. Then M3 fixes them.

CONSTRAINT: pkg/meet JobRef doc states "There is deliberately no second progress
channel: a job that reported itself somewhere else would be a second truth about what
happened in the room." Any fix extends the EXISTING live channel. No new store, no
new transport, no second channel.

MVP RESULT 2026-09-05: PASS, with no production M3 defect reproduced. The
command-synergy gate passes 22/22 with a freshly built Bashy binary. The real
Meet browser suite passes 29/29, including human send, working indication,
bounded cumulative progress, final response, failure visibility, and two-agent
Chat isolation. Console verifydom passes independently. G1 (room counters) and
G2 (job-status read) are confirmed but deferred feature expansions; the MVP
uses the existing Chat live/transcript path. G4 is confirmed: delivery state is
rendered separately from execution progress. Evidence and disposition are
recorded in coreutils `docs/request-progress-reporting.md` at commits 092ee8a6
and 3c2d2d02.
