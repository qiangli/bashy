# Sprint 587 master execution plan: class satellite for Sunday

Review date: 2026-11-07. Class: Sunday 2026-11-08. Operator decisions:
KISS/MVP, existing features only, no new code; one owner/admin per
satellite (the owner pays for it); no multi-team satellites.

## Goal

A class satellite owned by the operator hosts the class. Each student
pairs their own outpost and by default shares only metadata (system info,
online status). The class chats in Klatch (class topic, 1:1, per-group
topics). Each of three small groups has its own whiteboard, brainstorm and
meet room. Agentic tools on student outposts reach a shared Bedrock-backed
LLM.

## Verified state (code and docs, 2026-11-07)

| Need | Status | Mechanism |
| --- | --- | --- |
| AWS satellite under the operator | works | S522 AWS Lightsail runbook (`bashy infra provision aws --satellite NAME`), budget guard. Risk: the master never deletes satellite rows, so a reused name may be stuck; check the master's list first. |
| Student pairs, shares only metadata | works with a procedure | A host paired through the operator's code is owned by the satellite owner in the satellite record, and only the satellite owner mints codes. Without opt-out, the owner reaches podman, ollama, files and meet on a fresh install without the student's OS password. Procedure: the student runs `outpost builtins set --service podman=off --service ollama=off --service ollama-pool=off --service files=off --service meet=off --shell off --desktop off --clipboard off` before pairing; the operator mints plain, unscoped codes (scoped codes auto-share services across the scope). |
| Class chat, 1:1, group topics | works | Klatch SSO + team mode on the operator host, shared with each student. |
| Group spaces | works via app invites | whiteboard/brainstorm examples and the meet service; access by the apps' own invite links (satellite-level sharing by a student is not available because of host ownership). Simplest: the operator hosts one board/session/room per group. |
| Shared LLM for agents | partial | Sprint 586 `bashy llm share --via cloudbox --satellite NAME` (per-peer `/k/<cred>` URL). Live Bedrock 200 still pending. Fallback: a local model on the operator host shared the same way. |

Membership on cloudbox is always mutual: each party's email must be on the
other's member list (automatic when an invitation is accepted). So every
student first accepts the operator's member invitation; shares (Klatch,
LLM door, group apps) reach a student only after that.

## Stories

| Order | Story | Exit |
| --- | --- | --- |
| C1 `0b9dd209` | provision the class satellite | HTTPS up, owner sign-in, listed under the operator |
| C2 `ea2d84fc` | student onboarding runbook (install, opt out, pair, verify) | a test account shows metadata only |
| C3 `56c35911` | class Klatch | SSO, class topic, 1:1, group topic visible to its members only |
| C4 `2c88c5f7` | group spaces | one group end to end with test accounts |
| C5 `5b0a2a2b` | shared LLM | live 200 from a student's agent |
| C6 `235472ea` | dry run with 2-3 test accounts | all of the above on separate machines |

C1 first; C2 and C3 in parallel; C4 and C5 after C2; C6 last (Saturday).

## Limitations recorded (not fixed this sprint)

- Paired hosts are owned by the satellite owner in the satellite record;
  students cannot create satellite shares. Follow-up: student-owned
  pairing (todo `d969a9b5`, hosted satellite repository).
- whiteboard/brainstorm identity is per browser plus invite links, not
  satellite SSO; brainstorm sessions are open to anyone with the link.
- meet is text rooms, not video.

## Stopping rules

- Never pair a student host before the opt-out is applied.
- No code changes; anything that needs code goes to follow-up.
- Hosts by role only; no account addresses or satellite endpoints in this
  repository.
