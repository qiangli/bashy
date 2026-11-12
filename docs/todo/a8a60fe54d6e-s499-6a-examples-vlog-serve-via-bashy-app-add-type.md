---
id: a8a60fe54d6e
kind: feature
title: 'S499.6A examples/vlog: serve via ''bashy app add --type site'' and publish the console path through Outpost; delete the bundled Go server'
seq: 3227
status: done
priority: p1
labels:
    - app
    - web
    - examples
created: 2026-11-12T18:20:38.261849Z
weave: 103
assignee: opencode-glm-5.3
sprint: 499
sprint_id: c3c45f32-63b5-5575-980f-299a44a561f3
sprint_title: 'Content creation with bashy: YC videos and a local-first author-to-reader showcase'
closed: 2026-11-12T18:55:04.282577Z
closed_by: claude-opus5.5
---

S499.6 slice A, adoption. bashy now serves a static directory itself: `bashy app add NAME --type site --source DIR` (yoke 40d456db, bashy pin 0918ed7a). examples/vlog drops its own server.

Change examples/vlog (vlog.sh, dag.md, README.md):
- Delete server/ (go.mod + main.go), the `server` and `server-windows` targets, the bin/vlog-server build, and every mention of the ~30-line Go server and its Windows cross-build.
- `serve`: register the built site with the console instead of running a server.
  - Command: `bashy app add "${NAME:-vlog}" --type site --source "$SITE/public"`, then print the console URL http://127.0.0.1:${CONSOLE_PORT:-22749}/<name>/.
  - Re-running is idempotent: use `app set` if the app exists, with the source path unchanged.
  - If the console is not running, say how to start it (`bashy app serve`); never start a background daemon silently.
- `publish-outpost NAME`:
  - `bashy app set NAME --auth public` (an outpost public link reaches the console with no login)
  - then `outpost apps add NAME --url http://127.0.0.1:${CONSOLE_PORT:-22749}/NAME/`
  - The README must say plainly that this makes the site public, and how to undo it (`outpost apps rm NAME`, `bashy app set NAME --auth system`).
- `publish-pages` (GitHub Pages) is unchanged.
- The same commands work on macOS, Linux and Windows. No cross-build and no OS-specific branch.
- The example stays an example (examples are custom commands, not core). No hostnames, user names or addresses in the example (public repo).

Tests:
- Extend the example's existing test or dag `sample` target. Build the sample site, register it in a sandboxed BASHY_HOME (sandbox BASHY_COMMANDS_DIR / app ring), and check `bashy app show NAME` reports type site with the right source.
- Do not start long-running servers in tests. Live serving and outpost publishing are verified by the conductor in S499.4's re-run.
