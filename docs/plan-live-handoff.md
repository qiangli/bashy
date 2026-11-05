# Live handoff: offer/accept transfer of sprints, stories and claims

Planning only. This doc files the sprint and its stories. It authorizes no
implementation, and filing it starts no execution. Motivation:
`retro-s562-manager-handoff.md`.

## Goal

Let anyone, human or agent, hand over a sprint, story, todo, claim or run to
another human or agent with a live, two-sided handshake. The outgoing party
offers a generated snapshot plus a brief. The incoming party accepts (or
declines). On accept, ownership moves atomically. The outgoing party stays
accountable until the accept, so there is never a window with no owner.

KISS/MVP: one small generic verb over existing references, with `--json` for
composition (unix philosophy). Sprint and story workflows get sugar on top. It
reuses existing records and transports (sprint card and thread, todo store,
claims, weave links, MB/bus, `inbox human`). It adds no new transport and no
identity model.

## Relationship to other sprints

| Area | Owner |
| --- | --- |
| Identity, accounts, addressing, cross-service delivery (design) | Sprint 568 (D1-D6, `plan-s568-master-execution.md`) |
| Seat, inbox and clone/parent claim defects (F9-F9d) | Sprint 563 |
| Local handoff mechanics in bashy | this sprint |

The vocabulary follows Sprint 568:
- **actor** and **represented user** — a human working through an agent records
  both; neither replaces the other.
- **durable destination** — `sprint:<uuid>` role, story, todo, claim.
- **delivery acknowledgement** is distinct from **acceptance**.
- **Same authorization for humans and agents** — `person:<name>` can offer and
  accept exactly as an agent can.

H1-H3 are local and additive. They keep existing MB, Meet, claims and sprint
behavior, as Sprint 568 requires. H4 adopts Sprint 568's routing contracts once
D3/D4 exist, and does not invent a separate local addressing scheme.

## Proposed surface

```
bashy handoff show   <ref>  [--json]                 # snapshot of what would move
bashy handoff offer  <ref> --to <who> [-m brief]     # durable offer + delivery
bashy handoff accept <offer> | decline <offer> -m why
bashy handoff status [<offer>]                       # offered/delivered/accepted/declined/expired
```

- `<ref>`: `sprint:N|uuid`, `story:<id>`, `todo:<id>`, `claim:<name>`, `run:<repo>#N`.
- `<who>`: an agent name, instance, session id or `person:<name>`.
- Sugar: `sprint handoff N --to WHO` creates an offer; `sprint take N --offer ID`
  accepts it. Without `--to`, the existing behavior is unchanged.

## Stories

| # | Kind | Story | Depends on |
| --- | --- | --- | --- |
| H1 | feature | Handoff snapshot (`bashy handoff show`) | — |
| H2 | feature | Offer/accept/decline/status records and verbs | H1 |
| H3 | feature | Atomic transfer on accept | H2 |
| H4 | feature | Conform offers and acknowledgements to Sprint 568 routing contracts | S568 D3+D4, S563 F9-F9d |
| H5 | test/doc | End-to-end handoff scenarios and skill doc | H3 (H4 for routed variants) |

Acceptance details live in each story. Sequence: H1, H2, H3 locally; H4 after
Sprint 568 D3/D4 and the Sprint 563 fixes; H5 is the final verification.
