# Sprint 588 master execution plan: external defaults

Operator principle (2026-11-07): scripts, skills and kb pages live outside
the bashy binary; the only built-in is bashy's own bootstrap skill.

## Design

- Content lives in the bashy source tree: Bash# skill/scripts in
  `examples/*`, curated generic kb pages in `kb/defaults/`.
- Every install fetches that tree at its release pin
  (`bashy src fetch bashy` → `${BASHY_HOME:-$HOME/.bashy}/src/bashy`).
- D1 (yoke): `kb search|list|show` and `recall`/`context` read
  `src/bashy/kb/defaults` as a read-only ring after the writable rings
  (`$BASHY_KB_DEFAULTS_DIR` overrides); a local page with the same slug
  wins; nothing is ever written there.
- D2: `examples/defaults/bashy-defaults.bsh` (command + skill) fetches the
  pin, registers the default commands from it without overwriting a
  user's own registration, and reports the kb pages; the built-in skill
  points to it.
- D3: nine verified, generic pages curated into `kb/defaults/`.
- Bootstrap: `bashy self install` (alias `bashy bootstrap`, run by the
  public installers) fetches the pin and applies the defaults on a FIRST
  install only; an upgrade or reinstall with the pin present skips it
  (operator decision). Best effort, never fails the install;
  `--no-defaults` / `BASHY_SELF_NO_DEFAULTS=1` opt out.

## Stories

| Story | Delivery |
| --- | --- |
| D1 `d69e3577` | yoke f4dcd018; bashy pin 2dcab3ef |
| D2 `67a714d2` | examples/defaults + built-in skill pointer |
| D3 `40ae19f4` | kb/defaults (aa76ef56) |

Everything reaches users with the next Bashy release (the rc.27 pin
predates it).
