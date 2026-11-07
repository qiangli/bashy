# Sprint 587 master execution plan: class satellite for Sunday

Review date: 2026-11-07. Class: Sunday 2026-11-08. Operator decisions:
KISS/MVP, existing features only, no new code; one owner/admin per
satellite (the owner pays for it); no multi-team satellites.

## Goal

A class satellite owned by the operator hosts the class. Each student pairs
their outpost with their OWN cloudbox account (they own it), becomes a
mutual member with the operator, joins the class satellite as a member, and
shares only what they choose; minimal system info and online status visible
to sharing partners is acceptable for debugging (a trusted relationship). The class chats in Klatch (class topic, 1:1, per-group
topics). Each of three small groups has its own whiteboard, brainstorm and
meet room. Agentic tools on student outposts reach a shared Bedrock-backed
LLM.

## Verified state (code and docs, 2026-11-07)

| Need | Status | Mechanism |
| --- | --- | --- |
| AWS satellite under the operator | works | S522 AWS Lightsail runbook (`bashy infra provision aws --satellite NAME`), budget guard. Risk: the master never deletes satellite rows, so a reused name may be stuck; check the master's list first. |
| Student owns their host, shares by choice | works (own-account path) | The student pairs with their own account on the main cloudbox, so they own the host and nothing is reachable by others unless shared. Membership with the operator is mutual (invite + accept). The student joins the class satellite as a member (delegated satellite login) for the class apps. Student hosts are NOT paired with the class satellite: pairing through the satellite owner's code makes the satellite owner the host owner (checked in code), so that path is not used. Optional hardening: `outpost builtins set --service podman=off ... --shell off --desktop off --clipboard off`. |
| Class chat, 1:1, group topics | works | Klatch SSO + team mode on the operator host, shared with each student. |
| Group spaces | works | Leaders run the whiteboard/brainstorm examples and meet on their own outpost, publish with login + trusted identity, and share them with their group's members as host owners. Fallback: the operator hosts one board/session/room per group. |
| Shared LLM for agents | partial | Sprint 586 `bashy llm share --via cloudbox --satellite NAME` (per-peer `/k/<cred>` URL). Live Bedrock 200 still pending. Fallback: a local model on the operator host shared the same way. |

Membership on cloudbox is always mutual: each party's email must be on the
other's member list (automatic when an invitation is accepted). So every
student first accepts the operator's member invitation; shares (Klatch,
LLM door, group apps) reach a student only after that.

## Stories

| Order | Story | Exit |
| --- | --- | --- |
| C1 `0b9dd209` | provision the class satellite | HTTPS up, owner sign-in, listed under the operator |
| C2 `ea2d84fc` | student onboarding runbook (own-account pairing, mutual membership, satellite member, share by choice) | a test account owns its host; the operator sees only what is shared plus minimal system info |
| C3 `56c35911` | class Klatch | SSO, class topic, 1:1, group topic visible to its members only |
| C4 `2c88c5f7` | group spaces | one group end to end with test accounts |
| C5 `5b0a2a2b` | shared LLM | live 200 from a student's agent |
| C6 `235472ea` | dry run with 2-3 test accounts | all of the above on separate machines |

C1 first; C2 and C3 in parallel; C4 and C5 after C2; C6 last (Saturday).

## Limitations recorded (not fixed this sprint)

- whiteboard/brainstorm identity inside the app is per browser plus invite
  links; brainstorm sessions are open to anyone with the link.
- meet is text rooms, not video.
- Student hosts appear on the main cloudbox, not on the class satellite.

## Stopping rules

- Never pair a student host with the class satellite itself; students pair
  with their own account and join the satellite as members.
- No code changes; anything that needs code goes to follow-up.
- Hosts by role only; no account addresses or satellite endpoints in this
  repository.
