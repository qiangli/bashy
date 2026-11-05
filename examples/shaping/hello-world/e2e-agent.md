# Hello World agent E2E task (Sprint 564 H7, scenario A)

Run against the disposable installation prepared by the operator. Use the
configured Bashy executable and the selected interface for all shaping operations.
Do not edit the deployed app, bypass verification, or modify shared checkouts.
Do not expose credentials in prompts or logs.

1. Discover the selected shaping interface and supported authorization/scope options.
   If an operation is absent, report BLOCKED with the exact missing dependency.
2. Inspect hello-world status and record the source and active version identity.
3. In a real browser, confirm the initial heading is exactly Hello, World!.
4. Invoke the selected interface to request: Change the displayed greeting from "Hello, World!"
   to "Hello, Shaping!". Inspect the returned candidate and diff.
5. Verify through the selected interface. On failure stop; never approve a failed candidate.
6. Preview using the supported interface/browser route. In a real browser assert Hello, Shaping! in preview
   and Hello, World! in the active app. Retain browser assertion evidence.
   If either browser assertion fails or is unavailable, stop before approval and report FAIL or BLOCKED.
7. Explicitly approve through the selected interface. Assert Hello, Shaping! in the active app,
   the matching active version, and Hello, World! for a second principal.
8. Undo through the selected interface. Assert Hello, World! and restored version identity.
9. Submit the retained approved version through the selected interface using
   its automatically linked TODO, or an optional operator-supplied disposable
   story. Inspect tracking, origin, actor/represented user,
   source identity, verifier evidence, and reviewable proposal/run. Never merge.

Report each step as PASS, FAIL or BLOCKED with supporting commands, sanitized
output and artifact references. Missing browser access or a writable disposable
tracking store blocks the corresponding steps; no extra sprint/story is required
when the ordinary auto TODO supports submission. Narration is not browser evidence. Include
Bashy/source revision, agent/model version and platform. An overall PASS needs
all steps and independent review of the evidence. This task covers one H7 cell;
it does not establish other interfaces, other agents, negative cases or E2E B coverage.
