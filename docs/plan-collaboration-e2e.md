# S513.6a collaboration E2E plan

Run `python3 tests/collaboration-e2e/run.py` from this checkout. Set
`PLAYWRIGHT_MODULE` to an already installed Playwright module file and
`CHROMIUM_PATH` to an already installed Chromium executable. The gate needs
local `bashy`, Go 1.27+, Node, Python, and Bashy's signed tuwunel already
installed. It performs no downloads; Go builds with module and toolchain
network access disabled. Missing prerequisites return `BLOCKED` (exit 2).

The entry point builds the two native servers into a temporary directory and
starts the three existing independent E2E runners at once. Each runner keeps
its own temporary data and holds its live service at an opt-in barrier after
its full scenario. The gate verifies three distinct loopback ports and reads
each persisted state, then releases Matrix, whiteboard, and brainstorm in
turn. After every stop it checks the stopped endpoint is down and each
remaining endpoint still serves the expected state. Any failed assertion or
runner exit is a failure (exit 1). The barrier is inactive when a runner is
launched alone.

The Matrix runner exercises two real logins, invitation and DM, bidirectional
text, image upload and retrieval, setup idempotence, and restart. The
whiteboard browser runner creates a board, draws, adds notes, verifies browser
sync and restart, and checks PNG and JSON exports. The brainstorm browser
runner creates a session, checks private writing, reveal, vote budget, close,
and JSON and Markdown exports. This is an automated local gate; real lern
student LAN and remote UAT remains a separate manager-run check.

Live execution waits for the Matrix safety repair and Cloudbox integration
to land and for the manager to release the host-use hold. Local syntax and
offline prerequisite checks can run before then.
