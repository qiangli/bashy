# bashy docs

Start at [INDEX.md](INDEX.md), which has one entry per load-bearing doc.

## Abstracts

- **[Boundary control](boundary.md).** Everything on an owner's host belongs to
  the owner. Nothing is labelled or partitioned, and nothing leaves for another
  person without an explicit grant. A grant names a member email or a circle,
  and a circle is the attribute the owner gives each member on their Cloudbox
  member list (`family`, `friend`, `team`, `guest`; `public` means anyone).
  Circle grants resolve at check time, so removing a member revokes their
  access. Secrets are used by handle, redacted at egress, and revealed only at
  an interactive TTY. Checks run only at egress points: the LLM door, cross-host
  mb, shared meet, handoff, feed and public git push.
