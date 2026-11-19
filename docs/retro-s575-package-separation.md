# Moving the boxes: a package-separation anecdote

Sprint 575 was supposed to separate source ownership across repositories with
minimal package-reference changes. It grew into days of engine extraction,
test relocation, review handoffs and corrections to old harness assumptions.
The operator repeatedly asked for the smaller job.

Afterward:

> **Operator:** “I now understand human divorces are costly.”
>
> **Agent:** “Especially when the person hired to move the boxes starts
> redesigning both houses.”

The joke describes the failure: the agent expanded the scope and did the work
in the wrong order. More agents and more gates did not correct that decision.

Lessons for the next separation:

1. **Freeze the requested scope.** Source moves and package references are a
   concrete delivery. Shared-engine redesign belongs to a separate task.
2. **Move first, then build.** Use the prescribed build image, resolve the
   compiler's broken references, and run the existing binary entry points.
3. **Preserve the baseline.** Keep implementation, fixtures and assertions
   intact. Investigate a failure against the original source and venue before
   proposing any additional fix.
4. **Adapt existing test setup.** Reuse the proven runner and scope; update
   artifact paths and provenance for separate binaries. Verify actual runtime
   utility routing as well as imports.
5. **Keep handoffs concrete.** Send the exact patch and evidence promptly.
   Give launch estimates from the remaining work and resource availability.

Deliver the requested move before proposing a renovation.
