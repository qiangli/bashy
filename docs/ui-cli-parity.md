# UI–CLI parity: every web-console action has a bashy verb

Sprint 606 (bashy part). Bashy is built for agentic tools to easily discover,
use and manage its features through the CLI. Humans can run the same CLI;
human-friendly interaction is provided by the web apps and consoles.
Every feature and management action available in the web UI must have a
companion bashy verb tailored to agents (always `--json`, always `--dry-run`
on writes), with equivalent permissions and approval rules.

Agents may also use `bashy browser --live`, REST APIs or web URLs to inspect,
test or operate the web UI. These are valid alternative access paths, not
substitutes for missing CLI functionality: agents must not be forced to use
browser steps or ad-hoc HTTP because a companion bashy verb is absent.
CLI parity is mandatory; browser and API access remain allowed.

This page maps each console action to its verb.
Rows marked **gap** have no verb; each is filed as a standalone todo
(`--no-sprint`, label `parity`) and linked from the row.

Names are generic: no hostnames, addresses, account emails, or team
details. Console pages are named by role (Hosts, Members, Satellites,
Apps), never by URL.

Conventions: `bashy …` acts with the account credential; `bashy outpost …`
fronts the paired outpost service executable and acts with the host's
cloudbox pairing. The member journey composes these verbs in the
`satellite-member` Bash# skill-script
(`examples/satellite-member/satellite-member.bsh`), which is the model for
owner and member flows alike.

## Cloudbox portal (master account console)

| Console action | bashy verb |
|---|---|
| Sign in / sign out, account status, open the portal | `bashy tessaro` (status, sign in/out, open portal); `bashy login` pairs the machine |
| Hosts page: list hosts | `bashy paired-host ls --json` |
| Hosts page: generate a host pairing code (own account) | `bashy paired-host invite [--dry-run] --json` — pair-once, single use, 15 minutes; the host becomes owned by the minting account |
| Members page: list members | `bashy member ls [--circle C] --json` |
| Members page: invite a member (email + circle) | `bashy member invite EMAIL [--circle C] [--dry-run] --json` |
| Members page: change a member's circle | `bashy member set EMAIL --circle C [--dry-run] --json` |
| Members page: remove a member | `bashy member rm EMAIL [--dry-run] --json` |
| Members page: check one address | `bashy member check EMAIL --json` |
| Reverse view: who lists me | `bashy member of --json` |
| Member side: list pending invitations | `bashy member invitations --json` |
| Member side: accept an invitation (mutual) | `bashy member accept ID [--circle C] --json` |
| Satellites page: list satellites | `bashy satellite list --json` |
| Satellites page: open a satellite's console | human browser step; the agent equivalent is `bashy satellite show NAME --json` (endpoint URL). Not verb-shaped, not a gap |
| Satellite detail: show one satellite | `bashy satellite show NAME --json` |
| Satellite detail: mint an owner node-provisioning invite | `bashy satellite invite --satellite URL --name HOST [--to-file PATH] --json` — owner hardware only, never for members |
| Satellite detail: transfer ownership | `bashy satellite transfer NAME --to EMAIL --yes` |
| Migration preview | `bashy reproduce --plan` (read-only; no `--apply`) |
| Master credential for migration | `bashy fleet master-token save\|mint` (0600 file, never printed) |
| Hosts page: share an app with chosen members (share grant) | `bashy outpost apps share APP --email E` / `unshare` / `shares` on the host (S606 S7: the host's own token on its satellite or standalone; target must be an active mutual member); journey: `satellite-member share APP --email E` |

## Satellite web console

| Console action | bashy verb |
|---|---|
| Sign in as a member | human browser step; the agent path is pair once (`bashy paired-host invite` + `bashy outpost register --code`), accept (`bashy member accept`), attach (`bashy outpost satellite attach`), then open the app URL. Not verb-shaped, not a gap |
| View attached hosts and members | master view: `bashy paired-host ls --json`; this daemon's attachments: `bashy outpost satellite list --json` |
| Satellite-scoped host list (which hosts are attached to satellite X, from any machine) | `bashy paired-host ls --satellite ID --json` (Bearer `GET /api/v1/satellites/assignment`, S606 S5) |
| Reach a shared app | `https://<satellite>/matrix/h/<host>/app/<app>/` after attach + member sign-in; 200 for a shared member, 403 for an unshared member, 401 without a credential |
| Share an app with members | `bashy outpost apps share APP --email E` / `unshare` / `shares` on the host (S606 S7: the host's own token on its satellite or standalone; target must be an active mutual member); journey: `satellite-member share APP --email E` |

## Outpost admin (host-local service surface)

`bashy outpost …` passes through to the paired outpost service executable.
Single-daemon S606 contract: the daemon pairs ONCE with the cloudbox
portal (identity only, no tunnel) and takes satellites as attachments.

| Console/admin action | bashy verb |
|---|---|
| Pair this host (register with a code) | `bashy outpost register --code CODE [--name HOST]` |
| Unpair this host | `bashy outpost unpair` |
| List satellite attachments | `bashy outpost satellite list --json` |
| Attach to a satellite | `bashy outpost satellite attach SAT [--json]` |
| Detach from a satellite | `bashy outpost satellite detach SAT [--json]` |
| Peers | `bashy outpost peers` |
| Daemon status | `bashy outpost status [--json]` |
| Publish a local app | `bashy outpost apps add APP --port PORT` (then `bashy outpost apps share APP --email E`) |
| List / remove apps | `bashy outpost apps list`, `bashy outpost apps rm APP` |
| Built-in services on/off | `bashy outpost builtins set --service NAME=off …` |
| Remote shell / files | `bashy outpost connect`, `bashy outpost ssh\|scp` |

Pin note: the outpost binary bashy fronts trails the S606 contract in
older pins (no `satellite` subtree). The rows above name the contract;
**gap** tracks verifying them against the shipped pin (see below).

## LLM sharing (CLI-first; console rows for the reverse direction)

The LLM door has no web console; these rows exist so the reverse lookup
(console → CLI) also answers for sharing. Owner flow: `bashy llm peer add
NAME [--share allow] [--email E --circle C]`, `bashy llm peer share NAME
allow|deny`, `bashy llm peer limit NAME …`, `bashy llm peer revoke NAME`,
`bashy llm usage`. Member binding (`--email/--circle`) is checked on every
request: 200 for a shared member, 403 for an unshared member, 401 without a
credential; legacy peers without an email are unchanged.

## Gaps (filed `--no-sprint`, label `parity`)

- **parity-outpost-pin** (`todo:ea48d4aa81eb`): verify `bashy outpost
  satellite attach|detach|list` (plus `register`, `unpair`, `peers`)
  against the shipped outpost pin and record the minimum pin in this doc.
