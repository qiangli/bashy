---
id: 01a21103-40ec-72a3-b812-4a16d944932a
seq: 35
form: page
type: decision
title: 'Describe bashy: lead with shaping, claim only what ships'
description: 'How anyone writes about bashy (README, website, app description, announcement, kb page, pitch): lead with software shaping, then why now, then how; claim only what ships; exact certification and trademark wording; write for every owner and their members — WHEN writing or reviewing any text that describes bashy or software shaping (keywords: pitch, messaging, claims, positioning, readme, website, copy, announcement, certification wording, trademark, for everyone)'
tags:
    - guidelines
    - pitch
    - claims
    - shaping
status: candidate
evidence: Compiled 2026-11-24 from bashy README, docs/software-shaping.md, docs/public-claims-evidence.md and the founder narrative rule of 2026-11-12; DRAFT for operator review
source:
    tool: claude
created: "2026-11-25T04:42:35Z"
---

## Rules

Anyone writing about bashy follows these: a README, a website, an app's description, an announcement, a kb page or a pitch.

1. **Lead with software shaping.** Open with what it does for the reader, in words a non-engineer understands: you tell an agent in plain words how a running app should work, and you get your own verified, reversible version while everyone else keeps theirs. Undo is one step. The full definition is [software shaping](https://github.com/qiangli/bashy/blob/main/docs/software-shaping.md).
2. **Then why now, then how.**
   - Why now: agents are becoming the engineers, and the engineering process was built for people. Building got cheap; owning, verifying and updating what was built is still hard, and shaping is that layer.
   - How: bashy, one Bash-compatible shell and userland on every OS, and Bash#, where one file is both the program and the agent's skill.
   - The other features (the signed core, `--json` contracts, scoped activation, provenance) explain why shaping is safe. They are not a feature list.
3. **Claim only what ships.** Every public promise maps to one of:
   - code in a release;
   - a journey someone ran and observed;
   - a primary source;
   - a hypothesis, labeled as untested.

   Nothing on a roadmap is written in the present tense. A dated measurement stays dated: never round it into a current percentage.
4. **Say exactly what is and isn't certified.** bashy is "built for POSIX certification, not yet certified". "UNIX" is a trademark of The Open Group, so write "Unix-like", or use it only as an analogy.
5. **Write for everyone.** bashy is for anyone who owns a computer and the people they work with: a family, a team, a company with its staff and contractors. Write for an owner and the members they invite, not for one customer.
6. **Describe people and machines by role.** Write "the owner's host" or "a support member". Never a real machine name, login, address or credential.
7. **Treat objections as hypotheses.** When someone doubts shaping, attach a cheap test to the doubt instead of arguing it or accepting it as a verdict.

## Check before you publish

- Does the first sentence say what shaping does for the reader?
- Can every claim be traced to a release, an observed run or a source? Is the rest marked untested?
- Would a family, a team and a company owner each recognise themselves?
