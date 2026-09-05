---
id: 2371227693f5
kind: task
title: 'Human lane S1: one human identity across mb, meet, inbox and dm'
seq: 209
status: todo
priority: p0
created: 2026-09-05T15:47:51.775332Z
sprint: 126
---

Today 'bashy inbox' refuses without --as and points at 'qiangli'; 'bashy inbox human' keys off the OS user; 'meet dm --as' defaults to BASHY_AGENT_ID which a human does not have; and 'bashy todo add --owner qiangli' is REFUSED as 'not a registered agent' (reproduced 2026-09-05). Resolve to ONE human principal that every surface accepts. Gate: 'bashy whois human' resolves one name; that name posts on mb, is addressable by 'meet dm', can own a todo, and its unread appears exactly once in 'inbox human' (one cursor, no duplicate row). Test pins that an agent read cannot consume human state.

Implementation home: `bashy/internal/agentos` for principal/front-door wiring and
`coreutils/pkg/{bus,meet,weave}` for the shared identity consumers. Do not add an
identity service, account database, or dependency outside the dhnt umbrella.

VALIDATION 2026-09-05 (sprint scope audit) — the browser half of this story's
gate is currently UNREACHABLE, and no story named it. `bashy apps serve` and the
`meet` web surface derive the human DIFFERENTLY:

  webconsole  api.go userOf():   cloud -> Identity.Username, else session cookie,
                                 else coopauth.SystemUser(); THEN canonicalized
                                 through bus.BoardIdentity (panel_inbox.go:124,
                                 panel_mb.go:88).
  meet        serve.go actorOf(): cloud -> Identity.User, else WHATEVER THE
                                 REQUEST BODY STATES, else humanName() ($USER);
                                 never canonicalized.

Three concrete breaks: (1) Identity.User is the EMAIL and Identity.Username is
the derived app handle, so one cloud human is `liqiang@gmail.com` in meet and
`qiangli` in mb/inbox — two names, one person, so "whois human resolves one
name" cannot hold; (2) meet trusts a body-supplied sender, which webconsole
explicitly refuses ("a browser that could name its own from could sign as any
agent on the host", panel_mb.go:225-237); (3) meet never canonicalizes through
the fleet catalog. Fix is RECONCILIATION, not a new surface: meet's actorOf
adopts userOf's precedence and BoardIdentity canonicalization, and stops
honouring a body-stated sender. Gate extends to: the same human posts on mb and
tells in meet under ONE name, from CLI and from the browser.
