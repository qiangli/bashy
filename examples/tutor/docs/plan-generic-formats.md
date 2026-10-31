# Tutor generic formats — Sprint 547, story 3023

Rework the accepted tutor and invitation stories around owner-defined formats.

1. Parse roles, owner steps, audience and tone; default to a generic session.
2. Carry format metadata from plan/prep through apply to status and nudges.
3. Render private/shared and named-role editions; derive server labels from roles.
4. Replace cohort-specific documentation and rehearsal with generic examples.
5. Require invite --count; retain owner configuration and local hosting.
6. Verify meeting/solo flows, rendering boundaries, Python suites, Go race suite,
   the rehearsal gate and the two requested vocabulary greps; commit with trailers.

Solo's sole role performs session work and counts toward progress. Applied metadata
is snapshotted in the existing run log. Content resolves its format from front
matter or a nearby format.md. No compatibility aliases are retained.

## Delivery evidence

- Tutor: `cd examples/tutor && python3 -m unittest discover -s tests -t . -v`
  — 56 tests, OK, including meeting, solo, multiple participant roles and the
  example educational format; both private-block forms and local prep formats.
- Server: `cd examples/tutor/server && GOWORK=off go test -race ./...`
  — PASS (`tutor-server`, 24.718s), including format role page/API labels.
- Klatch: `cd examples/klatch && python3 -m unittest test_provision`
  — 19 tests, OK; includes registered-command and script invite flag validation.
- `cd examples/tutor && bashy dag dag.md dry-run` — both no-agent and stub
  lanes PASS; one target OK, isolated stores and command/skill parity verified.
- Browser check PASS with Playwright installed outside the checkout and cached
  Chromium 1243: display, format-derived owner title, phone page, controls and
  LAN-address join. No live services or accounts used.
- `git diff --check` — clean.

Vocabulary checks cover forbidden cohort/host names. Domain role labels occur
only in the educational example format and its tests; other uses of class
are Python declarations or HTML attributes.

Session steps use the personal todo store. Test homes are temporary. Missing
completion criteria receive the format default or a generic outcome criterion.
Between steps, the status line reports completed progress.

The shared kit has no countdown or full-width action widget. Tutor uses the
existing heading and button elements without adding styles.
