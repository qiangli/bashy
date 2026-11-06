---
id: 407a3a9218a8
kind: bug
title: 'S576 friction: coordination overflow redelivered in a loop; worker tests announce to the real board'
seq: 3127
status: todo
priority: p2
labels:
    - sprint
    - post-mvp
created: 2026-11-06T18:23:51.210794Z
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

Observed 2026-11-06 managing S576: (1) the same coordination-overflow notice (mb 12987-12988, identical content_digest) was injected into the manager session dozens of times within minutes until the items were acked with bashy inbox; an unchanged overflow digest should be surfaced once. (2) a weave worker's unit tests (run bashy#27, story 561bac1b) posted 'sprint #1 Test sprint directive — created in backlog' from 'conductor' to the real message board twice (mb:12986, mb:12990): the test sprint store was isolated but the stage announcement was not. Sprint test helpers should default announce off / isolate the bus.
