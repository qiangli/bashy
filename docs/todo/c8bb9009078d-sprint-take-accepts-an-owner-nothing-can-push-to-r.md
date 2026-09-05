---
id: c8bb9009078d
kind: task
title: Sprint take accepts an owner nothing can push to — reconcile the rule with the durable inbox
seq: 220
status: todo
priority: p2
created: 2026-09-05T19:08:32.159129Z
sprint: 126
---

FOUND 2026-09-05 validating the sprint operating modes. This is a DOC-vs-CODE inconsistency to DECIDE, not obviously a code bug.

room.OwnerTransport documents TransportNone as: "nothing will carry mail to this name. Refuse the seat." `sprint take` does not refuse — reproduced: taking a sprint with an owner that has never had a session succeeds.

BUT the rule as written now looks WRONG, and that is the point of this card. Established empirically the same day: the bus stores mail regardless of liveness. deliveryState returns `queued` for a recipient merely behind and `unverified` when no cursor exists yet; an agent that had NEVER been live received an mb post and read it afterwards. mb is asynchronous like email; meet is the real-time surface.

So "nothing will carry mail to this name" is false for any registered principal — mail is carried and stored; what is missing is a live PUSH channel and therefore prompt attention. An external agentic CLI is live only at its turn boundaries, so refusing the seat whenever nothing is live would make external managers unusable, which is the mode the sprint most needs to support.

DECIDE ONE OF:
  (a) the rule is wrong — TransportNone means "no live push", not "undeliverable". Reword the constant and its comment; take keeps accepting; whois already says "mail QUEUES for its next read rather than being pushed" (fixed same day).
  (b) the rule is right for SEATS specifically — an accountable seat requires a live reader — in which case take must enforce it and every intermittent external manager needs a defined grace window, because otherwise the seat is refused between turns.

(a) is the likelier answer given the durability evidence, and it is the cheaper one: it is a comment and a constant name, no behaviour change. Whichever is chosen, the code and the doc must end up agreeing — today they do not, and a reader who trusts the comment concludes the seat is undeliverable when it is merely unattended.

NOT URGENT: nothing is broken at runtime today. whois now reports the truth, and take accepting the seat is the behaviour the external-manager mode needs.
