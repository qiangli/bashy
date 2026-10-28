# Shared whiteboard

An independent general-purpose LAN whiteboard under the repository's
permissive [BSD-3-Clause license](../../LICENSE). No accounts, containers,
database service, external web assets, or AI calls. Participants use a browser.
It is **not** a tab or component of brainstorm: its own command, skill,
server, default port (`18093`), URL shape and data directory. It starts and
stops independently with brainstorm and Matrix absent, and all three can run
concurrently on different ports.

`whiteboard.bsh` is the canonical Bash# body: an embedded `~~~skill` face,
a Go fence for status/verified backups, and orchestration of the native board.
The Go HTTP server and browser page are supporting resources. The `.sh` entry
point is a compatibility shim, not another implementation.

Register once from this directory:

```bash
bashy ./register.sh
bashy whiteboard serve
```

The same body can run directly, without registration:

```bash
bashy --bashsharp /path/to/bashy/examples/whiteboard/whiteboard.bsh serve
```

The command and executable skill are two orientations of that body:

```bash
bashy whiteboard status                       # command → Bash# body
bashy skill show whiteboard                    # embedded skill projection
bashy skill run whiteboard                     # skill → command → same body
WHITEBOARD_ACTION=backup bashy skill run whiteboard
bashy commands show whiteboard                 # executable source + typed args
```

The skill's default action is a finite, read-only service check, not starting a
second server. Its contract checks health after the selected action and records
an attestation. Registration projects skill prose and its canonical contract
from the `.bsh` source through existing verbs; no hand-edited catalog files or
core registration. Re-run registration after editing embedded guidance.

The optional DAG target is `bashy dag serve`; some Bashy versions buffer DAG
output, so the direct command above is preferred for immediately seeing links.
The first launch builds a small native Go server (Go 1.27+ required). The server
stays in the foreground; Ctrl-C stops it, or `bashy whiteboard stop` stops a
server started for the same data directory. Restart resumes the saved boards.

## Run a session

1. Keep the host awake and connected to the group's LAN. Open the printed
   facilitator link privately. Share a printed **Participant** LAN URL;
   `127.0.0.1` works only on the host. Use the same LAN URL for the
   facilitator page when copying its participant link (keep the facilitator
   fragment).
2. Create a named board per group or topic. Boards are persistent, isolated
   sessions: clearing or locking one never touches the others. The `main`
   board is open; every other named board is private — press **Create invite**
   once per participant and share each link privately. Each invite is consumed
   on first join; the code lives in the URL fragment, outside HTTP request
   paths. Anyone with only the
   board name (a guessed URL) is denied on state, draw/note, sync poll, and
   both exports until they join with a valid invite. Revoking a member or an
   invite takes effect immediately and survives restarts.
3. Draw freehand on the canvas and pin sticky notes. Every open browser polls
   and re-renders, so contributions appear everywhere within a couple of
   seconds; a dropped connection shows "Reconnecting…" and resumes.
4. Lock the board to freeze contributions while presenting; unlock to continue.
   Only the facilitator link can lock or clear — the server rejects anyone
   else, not just the buttons.
5. Export JSON or PNG for the record. Facilitator keys and browser ownership
   tokens never appear in state responses or exports.

Browser cookies identify participants; clearing cookies or using another browser
creates a new participant. Ownership limits suit a cooperative group, not a
verified election. Do not share the facilitator link. The host operator can read
the saved files.

Data defaults to `~/.local/share/bashy/whiteboard/boards` (respects
`XDG_DATA_HOME`). No automatic deletion. Start a fresh set of boards without
deleting the old ones:

```bash
WHITEBOARD_DATA="$HOME/.local/share/bashy/whiteboard/session-2" bashy ./whiteboard.sh
```

Set `WHITEBOARD_ADDR=0.0.0.0:18093` to use another port. Set
`WHITEBOARD_PREFIX=/whiteboard` when a proxy preserves that sub-path. Outpost
strips `/app/<name>` before forwarding, so leave the prefix empty there. Bind
to loopback and register with
`outpost apps add whiteboard --url http://127.0.0.1:18093`. Its HTTPS URL is
`https://<cloudbox>/matrix/h/<host>/app/whiteboard/`. Custom-app add/remove
are live operations; `outpost apps rm whiteboard` removes the route without
restarting the daemon. All page URLs are relative so the board keeps working
there. Host firewall rules and Wi-Fi client isolation can prevent guest access
even on the same Wi-Fi. Everything works without internet once Go and the
executable are available.

`bashy whiteboard backup` prints a verified snapshot directory. Stop the
server, choose a **new, empty** data directory, then run
`WHITEBOARD_DATA=/new/board-data bashy whiteboard restore /saved/snapshot`
before serving it again. Restore refuses nonempty destinations, invalid
records and overwrites. Snapshot JSON contains facilitator capabilities;
store it privately. Members and revocations survive restore.

Run focused tests with `bashy dag test`. There is no change to the Bashy binary.
Run `python3 verify-duality.py` after registration to check direct script,
registered command and attested skill against an isolated temporary board,
including equivalent status, byte-identical backup contents and rejection of
failed steps without a passing attestation.
For the optional browser test, install Playwright separately and set
`PLAYWRIGHT_MODULE` to its module path and `WHITEBOARD_BIN` to the built server,
then run `node browser-test.mjs`. `CHROMIUM_PATH` can select an existing browser.
