# Unified inbound communication

`bashy inbox` is the one receive-side surface for host-local agent
communication. It is a read-through view over existing durable sources:

- the public message board (`bashy mb`), including steward and conductor role
  addresses;
- standing Meet boards (`bashy meet ... --board`);
- addressed Bus notifications; and
- retained legacy role-pending buffers, drained for migration compatibility.

It creates no message store. Every source keeps its own cursor. A read follows
the same transaction shape for each source: snapshot unread records and their
high-water mark, render the complete combined batch, then acknowledge only the
rendered source watermark. `--peek` performs no acknowledgement. A failed
render leaves every source unread. `--limit` also leaves a capped source unread
rather than silently consuming records it omitted.

`--wait DUR` waits for one batch. `--watch` follows all sources until
interrupted; `--watch --wait DUR` gives it a total bound. `--json` emits
`bashy-inbox-v1` NDJSON with source, source sequence, sender, recipient, topic,
room, timestamp, and body.

## Turn-boundary delivery

A session launched and registered by Bashy has a verified agent identity and a
control socket. Its opening prompt and each subsequent `chat.Session.Say` turn
receive at most one combined inbox block before the caller's instruction. The
block is part of that model turn and therefore passes through the existing LLM
budget gate; it is not a free hidden call.

A third-party agent process started outside Bashy has neither an authenticated
room card nor a control socket. Bashy cannot safely steer it, and does not guess
from a PID or claim live adoption. Its reliable path is explicit pull:

```sh
bashy inbox --as <fleet-agent-name>
bashy inbox --as <fleet-agent-name> --watch
```

This is a reachability limit, not message loss: all inputs remain durable in
their original stores until that identity reads them.
