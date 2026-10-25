# Classroom brainstorming MVP

Sprint 506, Story 2d890260c745. Implement a native first-party example with a
Go standard-library server and embedded browser UI, launched by one Bashy
command. No containers, accounts, external database, CDN or AI service.

One persistent board has five challenge columns and teacher-controlled phases:
private writing, discussion, voting, closed. Private cards are filtered on the
server. Browser participants may edit their own cards during writing and cast
three votes, at most one per card. A separate random teacher capability controls
phases and export. Save state atomically before acknowledging mutations.

Use examples/brainstorm with a standalone Go module, launcher and DAG. Default
port 18091; print LAN student links and a separate teacher link. Store data under
the user's home outside the repository; Ctrl-C stops the foreground server.

Verify independent teacher/student sessions, privacy, ownership, voting limits,
concurrency, restart recovery, save failures, text escaping, export and 30
participants. Smoke through installed Bashy and verify reachability from another
LAN host. Deliver the running classroom link and stop/restart instructions.

## Delivery evidence

- Go race-enabled tests passed: privacy/ownership, teacher authorization,
  phase ordering, vote limits, persistence, 30 concurrent participants,
  failed saves, invalid input, cross-origin rejection, corrupt-file preservation.
- Real Chromium sessions passed: independent students/teacher, private API,
  editing, draft preservation across polling, reveal, HTML treated as text,
  voting/undo/limit, mobile viewport, close and JSON download.
- Installed Bashy launched the native example; a second LAN machine reached
  its health endpoint successfully. The classroom board remains empty in the
  private-writing phase; test boards were isolated and removed.
- The direct `bashy /path/to/examples/brainstorm/brainstorm.sh` command is the
  recommended entry point because the DAG runner buffers startup output.
- Example inherits the repository's BSD-3-Clause license; it does not bundle
  Scrumlr or any third-party service. No Bashy core rebuild is needed.
