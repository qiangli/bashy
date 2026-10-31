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

Requested vocabulary checks (run after staging new example formats and tests):

```sh
git grep -n -i -E 'lern|psic|week ?2|nov 1|classbot|novidesign|alien' examples/tutor examples/klatch
```

No output (zero matches, exit 1).

```sh
git grep -n -i -E 'teacher|student|class\b' examples/tutor examples/klatch
```

Output on this macOS host:

```text
examples/tutor/formats/class.md:4:- teacher
examples/tutor/formats/class.md:5:- student
examples/tutor/formats/class.md:6:audience: students aged 13 and up
examples/tutor/formats/class.md:19:pairing: pairs of two, rotate at every phase boundary; a lone student pairs with the teacher
examples/tutor/formats/class.md:34:This is an example session format: one teacher, a handful of students, one
examples/tutor/formats/class.md:40:- Every student step is something a person can finish inside its timebox and
examples/tutor/formats/class.md:45:  `work` is where the goal is actually attempted, `demo` is where students show
examples/tutor/formats/class.md:48:  leave the owner empty and the teacher assigns at the desk.
examples/tutor/formats/class.md:49:- The teacher's own steps (prepare / facilitate / review) belong on the card
examples/tutor/formats/class.md:51:- Honour the tone: the session is timeboxed so students can be stuck safely.
examples/tutor/tests/test_formats.py:111:        self.assertEqual(draft['roles'], ['teacher', 'student'])
examples/tutor/tests/test_formats.py:112:        self.assertEqual(draft['audience'], 'students aged 13 and up')
examples/tutor/tests/test_formats.py:114:        self.assertEqual(value['roles'], {'owner': 'teacher', 'participants': ['student']})
examples/tutor/tests/test_formats.py:115:        self.assertIn('students aged 13 and up', common.metadata(int(sprint))['audience'])
examples/tutor/tests/test_formats.py:118:        self.assertIn('Answer', self.tutor('render', str(item), '--edition', 'teacher').stdout)
examples/tutor/tests/test_formats.py:119:        self.assertNotIn('Answer', self.tutor('render', str(item), '--edition', 'student').stdout)
```

A separate plain-text search also checked the word `class`: remaining uses
outside the example format and its test are Python declarations and HTML/JS
attributes, which are language syntax. Word-boundary handling in the requested
grep expression varies with the host regex implementation.
