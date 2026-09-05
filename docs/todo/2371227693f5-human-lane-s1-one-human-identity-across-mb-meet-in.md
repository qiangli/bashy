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
