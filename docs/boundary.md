# Boundary control: owner by default, share by choice

Design of record for Sprint 583 (S583.1). Design only; the KISS rescope of
2026-11-09 replaces the earlier per-circle partition plan. Names below are
placeholders (`owner@example.com`, `member@example.com`), and hosts are named
by role.

## Model

1. **Everything on an owner's host belongs to the owner.** Nothing on disk is
   labelled, partitioned or migrated, and owner-only is the default. The
   requirement that the owner circle is absolute is met by refusing every
   member-facing egress that has no explicit grant. It is not met by fencing
   stores off.
2. **A circle is an attribute of a member, not of data.** It is the circle the
   owner gives each email on their Cloudbox member list: `family`, `friend`,
   `team` or `guest`. `public` means anyone. The Cloudbox list is the only source
   of truth (ticket a99e88ac: validated `circle` field, `GET /api/v1/members`,
   and a reverse "who lists me" view). bashy keeps no parallel list: no
   `circles.yaml` and no `BASHY_CIRCLE`.
3. **A share grant names an email or a circle.** A circle grant resolves to the
   circle's *current* active members at check time. Removing a member, or moving
   them to another circle, revokes their access at once, bounded only by the
   list's cache TTL. Inactive, pending, unknown and removed emails never match.
4. **Secrets never leave the owner.** Agents use secrets by handle only
   (`secret://…`, children get a scrubbed env via `secrets.ScrubAgentEnv`).
   Known values are redacted at egress (`secrets.Redactor`). Revealing a
   plaintext value requires the interactive TTY approve gate, and agent or
   non-TTY callers are refused.
5. **Enforcement happens only at egress points**, where data leaves the host for
   another person. Inside the host nothing changes.
6. **Agents on the owner's host act as the owner.** An agent that serves members
   (for example through the LLM door) acts for the member and sees only what was
   granted to that member.

The owner is the paired account email. An unpaired host keeps today's implicit
owner, `local` (`yoke/pkg/principal.LocalOwner`), and behaves exactly as it does
today. The member list, grants and the `bashy member` verb are specified in
S583.2.

## Egress inventory

"Today" means the code as of 2026-11-09.

| Egress point | Reaches another person today? | Today's check | S583 MVP | Follow-up (`boundary-later`) |
|---|---|---|---|---|
| Cloudbox app/host shares | Yes, the sharee emails | Per member email, enforced by Cloudbox | No change | — |
| LLM door (`bashy llm peer`, `llm share`) | Yes: a peer host, or anyone holding the `/k/<credential>` link with `--via cloudbox` | Per-peer `bpeer_` credential (stored as SHA-256), deny until `share allow`, 401 after revoke, inference routes only, S602 limits, captured turns redacted. No email binding | S583.11: a peer may be bound to a member email or a circle, checked on every request (403 with a plain reason for a non-member); legacy peers without an email are unchanged | Per-peer model grants (todo 0a13a9da) |
| Cross-host mb / bus (`mb:PERSON@HOST`) | Yes, through a shared Cloudbox weave session (`weave share EMAIL`, join-by-repo) | Session membership, enforced by Cloudbox. No sender-side check; body sent unredacted | S583.3 redacts the body; S583.11 adds the member check when the post reaches another account | S583.6 |
| `meet --shared` / room | `meet --shared`: yes, the same path as mb. Plain meet and room: local only | Session membership only | Same as mb | S583.7 |
| handoff | No network send; the record (brief plus diff) is carried by hand | None | S583.3 redacts the record, because it is built to travel | S583.13 |
| feed (git / rclone sinks, groups) | Yes, to followers. Groups with `read: members` are encrypted per member | Group member list; public sinks have no check; no redaction | S583.3 redacts posts | S583.13: grant check on non-public sinks |
| Public git push | Yes | The umbrella's `scrub-gate` pre-commit hook (pattern scrub, fail-closed) and CI `--all` | No change | S583.13: push-time check |

Device pairing (`webconsole pair`) admits the owner's own devices with a
device-scoped session. Those devices act as the owner, so device pairing is not
a member egress.

## Decisions

- **Owner by default.** This is the model above. An absent grant means no
  egress. There is no migration and no on-disk label.
- **The name is "circle".** It names a relationship to a person, which is what
  the Cloudbox field records. "Group" is already taken by feed groups and
  Cloudbox groups. "Tier", "zone" and "scope" read as properties of data or of
  the host, which is exactly what this model rejects.
- **Mutuality: accepting an invite adds the reverse row** (recommendation from
  ticket a99e88ac, accepted). When `member@example.com` accepts the owner's
  invite, the owner also appears on the member's own list. By default the
  reverse row carries the same circle, and the member may change it. Each side's
  circle is that side's own choice. A grant always checks the *granting* owner's
  list, so mutuality never widens what either side shares.
- **An offline host uses its last known list.** When Cloudbox is unreachable,
  the check uses the last successfully fetched list. An email not on that list is
  not a member. A host that has never fetched a list has no members, so it is
  owner-only. A removal made while the host is offline takes effect at the next
  successful fetch, and that delay is the accepted price of working offline.

## Stories (Sprint 583)

In scope: S583.1 (this doc), S583.2 (members and grants: `bashy member` over the
Cloudbox list, circle check, owner = paired email), S583.3 (secrets never
leave), S583.11 (share points check members: LLM door peers) and S583.15
(closure: scenario suite and no-regression evidence).

Deferred, unlinked from the sprint and labelled `boundary-later` (p3): per-circle
store partitions (S583.4, .5, .9), ceiling-locked `BASHY_CIRCLE` sessions,
per-circle credential and CLI profiles and model routing (S583.10), comms circle
partitions (S583.6, .7), fleet circle sync (S583.8), execution isolation
(S583.12), the remaining egress checks (S583.13) and observability (S583.14).

## Prior art

Human-lane 101f7ec3b611 (I14/I15), S513 invite-only sharing (the whiteboard
story; see `plan-matrix-chat-cloudbox-shim.md` and `plan-collaboration-e2e.md`),
and Sprint 606 pair-once.
