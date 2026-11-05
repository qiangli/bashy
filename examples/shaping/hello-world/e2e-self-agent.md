# Self-shaping agent E2E task (Sprint 564 H7, scenario B)

Use only the disposable installation and selected interface in the run context.
The operator must supply a clean pinned Bashy source checkout matching the
retained executable's embedded release identity, trusted CLI build/test argv,
and a disposable tracking story. An unknown release source blocks rollback
acceptance. Do not change shared sources, protected control code, build gates,
release/PATH installation, or credentials. A local OS account remains trusted;
this exercise does not establish a sandbox for malicious native code.

1. Discover the selected interface. Inspect Bashy target status, release source
   and artifact identity, host scope, actor and represented-user attribution.
2. Record the original `bashy shape --help` text and Hello World status.
3. Request exactly: Append "Software Shaping demo" to the visible short help
   description of `bashy shape`, changing only the editable help text file.
   Inspect candidate metadata and diff; protected paths must remain unchanged.
4. Verify through the selected interface; stop on failure. Preview the verified
   candidate with `shape --help` arguments using the supported preview surface.
   Record exact artifact identity and visible changed help without activation.
5. Explicitly approve. Invoke the normal `bashy shape --help` entry, assert the
   added words, and inspect host source/artifact identity and retained release.
   Use that shaped entry to inspect Hello World status and retain its result.
   Authoritative mutations must still route to the retained control plane.
6. Undo in one command. Confirm original help text and exact retained release
   artifact/source identity. No manual binary replacement or PATH edits.
7. Submit the retained approved candidate through the selected interface to the
   disposable story. Inspect reviewable proposal/run, origin, identities and
   verifier evidence. Never merge automatically.
8. Retain real browser evidence for the accompanying Software Shaping app/UI
   requirement and Hello World observation. CLI-only output does not close the
   app/UI requirement; report any missing app/UI proof explicitly BLOCKED.

Report each step as PASS, FAIL or BLOCKED with actual interface events, source
and binary identities, tool/model version, host role/OS and evidence references.
Missing preview, browser, clean release provenance or submission dependencies
are blockers, not passing cells. A zero agent exit code is not acceptance.
