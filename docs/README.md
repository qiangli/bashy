# bashy docs

Start at [INDEX.md](INDEX.md), which has one entry per load-bearing doc.

## Abstracts

- **[Sprint 614 collaboration evidence](s614-collaboration-evidence.md).**
  One final journey pass after all Sprint 614 features landed: per-tool
  results tied to the candidate commits, the Git LFS defect it found and
  fixed in collab-save, the pre-existing browser E2E failure, and what is
  absent or deferred (Memos, calendar build, deployed satellite reach).

- **[Memos decision](memos-decision.md).** Sprint 614 S7 assessed
  `usememos/memos` against the existing Notepad and KB for author-owned notes
  and recorded the smallest example-level adapter with full CLI parity. The
  operator parked it on 2026-11-10: no follow-up story is filed.

- **[Calendar decision](calendar-decision.md).** Sprint 614 S8 chose a tiny
  self-hosted `examples/calendar` built from Desk primitives over third-party
  calendars. Events are the source of truth locally; cloud calendars connect
  only through explicit ICS or Google import/export. The doc carries the CLI
  parity matrix, shaping hooks and the draft follow-up story.

- **[Boundary control](boundary.md).** Everything on an owner's host belongs to
  the owner. Nothing is labelled or partitioned, and nothing leaves for another
  person without an explicit grant. A grant names a member email or a circle,
  and a circle is the attribute the owner gives each member on their Cloudbox
  member list: any owner-chosen name, with `family`, `friend`, `team` and
  `guest` as defaults (`public` means anyone and `owner` is reserved).
  Circle grants resolve at check time, so removing a member revokes their
  access. Secrets are used by handle, redacted at egress, and revealed only at
  an interactive TTY. Checks run only at egress points: the LLM door, cross-host
  mb, shared meet, handoff, feed and public git push.
