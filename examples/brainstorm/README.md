# Classroom brainstorm

A first-party LAN brainstorming board under the repository's permissive
[BSD-3-Clause license](../../LICENSE). No accounts, containers,
database service, external web assets, or AI calls. Students use a browser.

`brainstorm.bsh` is the canonical Bash# body: an embedded `~~~skill` face,
a Go fence for status/verified backups, and orchestration of the native board.
The Go HTTP server and browser page are supporting resources. The `.sh` entry
point is a compatibility shim, not another implementation.

Register once from this directory:

```bash
bashy ./register.sh
bashy brainstorm serve
```

The same body can run directly, without registration:

```bash
bashy --bashsharp /path/to/bashy/examples/brainstorm/brainstorm.bsh serve
```

The command and executable skill are two orientations of that body:

```bash
bashy brainstorm status                       # command → Bash# body
bashy skill show brainstorm                    # embedded skill projection
bashy skill run brainstorm                     # skill → command → same body
BRAINSTORM_ACTION=backup bashy skill run brainstorm
bashy commands show brainstorm                 # executable source + typed args
```

The skill's default action is a finite, read-only service check, not starting a
second server. Its contract checks health after the selected action and records
an attestation. Registration projects skill prose and its canonical contract
from the `.bsh` source through existing verbs; no hand-edited catalog files or
core registration. Re-run registration after editing embedded guidance.

The optional DAG target is `bashy dag serve`; some Bashy versions buffer DAG
output, so the direct command above is preferred for immediately seeing links.
The first launch builds a small native Go server (Go 1.27+ required). The server
stays in the foreground; Ctrl-C stops it. Restart resumes the saved board.

## Run a session

1. Keep the host awake and connected to the students' LAN. Open the printed
   teacher link privately. Share a printed **Student** LAN URL with students;
   `127.0.0.1` works only on the host. Use the same LAN URL for the teacher page
   when copying its student link (keep the teacher fragment).
2. Give students 5–10 minutes to write. Nicknames are optional. Only the author
   sees their cards during this phase, including in API responses.
3. Select **Reveal all ideas**, then discuss the board. Revealing ends editing.
4. Select **Open voting**. Each browser participant gets three votes, one per
   card; clicking an existing vote removes it.
5. Select **Close voting** and download JSON or Markdown. Phase changes are
   irreversible. The teacher can export after reveal.

Browser cookies identify participants; clearing cookies or using another browser
creates a new participant, so vote limits are suitable for a cooperative class,
not a verified election. Do not share the teacher link. Private writing prevents
other browsers from reading ideas, but the host operator can read the saved file.

Data defaults to `~/.local/share/bashy/brainstorm/board.json` (respects
`XDG_DATA_HOME`). No automatic deletion. Start a fresh session without deleting
the old one:

```bash
BRAINSTORM_DATA="$HOME/.local/share/bashy/brainstorm/session-2" bashy ./brainstorm.sh
```

Set `BRAINSTORM_ADDR=0.0.0.0:18092` to use another port. Host firewall rules and
Wi-Fi client isolation can prevent student access even on the same Wi-Fi.
Everything works without internet once Go and the executable are available.

Run focused tests with `bashy dag test`. There is no change to the Bashy binary.
Run `python3 verify-duality.py` after registration to check direct script,
registered command and attested skill against an isolated temporary board,
including equivalent status, byte-identical backup contents and rejection of
failed steps without a passing attestation.
For the optional browser test, install Playwright separately and set
`PLAYWRIGHT_MODULE` to its module path and `BRAINSTORM_BIN` to the built server,
then run `node browser-test.mjs`. `CHROMIUM_PATH` can select an existing browser.
