# Live agent identity claims

## Goal

Prevent one local agent session from accidentally authoring coordination as a
different registered agent while preserving legitimate commands spawned by the
rightful session. This is a cooperative host-local identity boundary, not a
cryptographic security boundary.

## Contract

1. A live registered identity has one canonical room claim. Inbox watchers,
   interactive sessions, and other singleton sessions use the fleet name as
   input to the shared room claim-ID function; a second claimant fails before
   doing work even when the fleet name contains dots or other path punctuation.
2. A bare human inbox watch remains valid and does not mint an agent identity.
   An agent watcher must already exist in the fleet catalog; aliases resolve to
   the canonical name.
3. Authored MB post/send, Meet-board tell, ping, notify, Bus publish, and human
   inbox send paths consult one sender guard before append. If a different live
   session holds the requested registered name, the write fails closed and a
   short system notification is queued for the rightful owner.
4. `BASHY_PRINCIPAL` states the intended author but never proves ownership by
   itself. A registered author needs a live claim and either a matching hashed
   tool-session identifier or matching process ancestry. The watcher records
   the stable agent-parent PID as a fallback for tools without session metadata.
5. `bashy whois agent:NAME` exposes a live claim as `TAKEN`, including its mode,
   card ID, and PID. `bashy agents` continues to project the same room card and
   removes it when the watcher exits.

## Verification

Use isolated fleet, room, Meet, and MB stores. Cover registered-only watcher
selection, alias canonicalization, duplicate/global claim rejection, rightful
subprocess authorization through stable session and nested-wrapper ancestry,
forged-principal cross-agent refusal and notification for each
authored transport, `whois` claim output, active-roster visibility, and cleanup.
Run focused package tests, both repositories' full Go suites, and Bashy's
Windows cross-build.
