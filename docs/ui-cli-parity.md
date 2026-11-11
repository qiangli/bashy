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

## Sprint 614 S1: collaboration app and cross-tool audit

This is a **web/app → bashy CLI** audit of the checkout, not a claim that every
CLI feature needs a page. `bashy commands NAME --features` confirms installed
entries for klatch, tutor, kb, viewer, notepad, brainstorm, whiteboard, form,
record and transcribe; it reports `not-found` for chart, video and google on
this host. The checkout still contains `examples/desk/chart/register.sh` and
`examples/google/register.sh`; video is a script (`examples/video/video.sh`).
Registration is local and does not by itself prove a browser action works.
Evidence below is relative to this repository unless labeled as command output.

The shared grammar (`docs/command-atlas.md:480-526`) recognizes only its closed
kind list. The actual `define` wiring (`internal/agentos/refs.go:42-62`) covers
KB and sprint, but no app-specific desk, Klatch or media kind. A desk `id` or
URL is stable *within that app*; it is not automatically a `kind:id` that
`bashy define` can open. UUID/UID/slug/scoped sequence rules apply to the
owning KB and sprint stores, not automatically to the apps in this table.
`examples/desk/kit/README.md:18-38` defines the common seven verbs, item URL,
saved JSON and invite checks; each app's actual verbs and guards still decide
parity. `--json` is the scriptable result path for registered desk commands.

| Tool | Owning entity/store; ref emit/accept | `define` | Browser open/share; audience check | Output/export; Git | CLI parity and evidence |
|---|---|---|---|---|---|
| Klatch | Databag chat server; participant IDs/messages, no Bashy app ref (`examples/klatch/README.md:1-19`, `examples/klatch/klatch.bsh:19-32`) | No Klatch kind in `internal/agentos/refs.go:42-62` | External Klatch client; server credentials via `secret show`, argument secrets refused (`examples/klatch/README.md:28,64-67`) | Messages/JSONL `watch`; external server owns history; no app Git contract | Registered Bash# `klatch` (`examples/klatch/register.sh:6-14`); **S2 ref gap**, not evidence of missing participant CLI verbs |
| Tutor / Session | Sprint card is canonical `sprint:id`; tutor draft/session JSON is separate (`examples/tutor/README.md:111-125,168-170`) | Sprint resolves; tutor session does not (`internal/agentos/refs.go:48`) | `serve`/`join`/`render --edition ROLE|shared`; edition filtering (`examples/tutor/tutor.bsh:44-64`) | Rendered step; draft directory can be kept in Git (`examples/tutor/README.md:116`) | Bash# register lists plan/apply/run/render/serve/join (`examples/tutor/register.sh:6-20`); Session naming/compatibility belongs to S5, **S2 session ref gap** |
| KB | KB ring owns `kb:<slug>` plus UID and scoped sequence input; canonical slug ref emitted (`docs/command-atlas.md:508-526`) | Yes (`internal/agentos/refs.go:47`) | No KB browser catalog in Klatch/Viewer integration found; KB CLI reads own ring (`internal/agentos/refs.go:26-34`; `examples/desk/viewer/main.go:1-15`) | `kb search/show/context` JSON; repo KB files can be committed via `git` (`docs/command-atlas.md:131-143`) | `bashy commands kb --features`: available, JSON; **S3 browser catalog gap** |
| Viewer | Book ID in desk JSON (`examples/desk/viewer/main.go:13-15`); app-local ID/URL, no kind ref | No viewer resolver (`internal/agentos/refs.go:42-62`) | Loopback book page and invite; same CLI/API actions (`examples/desk/viewer/cli.go:1,181-242`) | HTML/PDF export (`examples/desk/viewer/viewer.bsh:25-27`); exported files can use `git` | `add/append/open/show/list/share/close/export`; view-only set refusal (`examples/desk/viewer/cli.go:217-242`); **S2 ref gap**, otherwise verified OK |
| Notepad | Item ID in desk JSON (`examples/desk/notepad/README.md:55`); app-local ID/URL, no kind ref | No notepad resolver (`internal/agentos/refs.go:42-62`) | Loopback edit/share; owner key and invite (`examples/desk/notepad/README.md:7-25`) | PDF/PNG/HTML/TXT export (`examples/desk/notepad/cli.go:431`); files can use `git` | `add/open/show/set/list/share/close/export/join` (`examples/desk/notepad/cli.go:326-471`); **S2 ref gap**, otherwise verified OK |
| Brainstorm | Named session in saved JSON (`examples/brainstorm/README.md:106-111`); local name/URL, no kind ref | No brainstorm resolver (`internal/agentos/refs.go:42-62`) | Page supports ideas, votes, invites/revoke, phase/round controls (`examples/brainstorm/server/index.html:147-159,182-188`); facilitator guard (`examples/brainstorm/server/desk.go:250-356`) | Browser JSON/Markdown export (`examples/brainstorm/server/index.html:187-188`); backup/restore, files can use `git` | Registered desk verbs and phase/round flags (`examples/brainstorm/server/cli.go:70-105,110-130`); **new CLI action gap** and **S2 ref gap** |
| Whiteboard | Named board in saved JSON (`examples/whiteboard/README.md:81-86`); local name/URL, no kind ref | No whiteboard resolver (`internal/agentos/refs.go:42-62`) | Page draws, locks, clears, invites/revokes; facilitator and participant checks (`examples/whiteboard/server/index.html:115-120`; `examples/whiteboard/server/desk.go:180-224`) | Browser JSON/PNG export; backup/restore, files can use `git` (`examples/whiteboard/server/index.html:119-120`) | Registered desk seven (`examples/whiteboard/server/cli.go:63-106`); **new CLI action gap** and **S2 ref gap** |
| Form | Form ID in desk JSON; no KB UUID/sequence (`examples/desk/form/README.md:162-175`) | No form resolver (`internal/agentos/refs.go:42-62`) | Loopback respondent answer/join and owner share; owner key gates show/set/share/close (`examples/desk/form/README.md:175-183`) | JSON answers via `show/wait`; saved files can use `git` | CLI has owner verbs, `reply` and a `join` that prints a URL (`examples/desk/form/server/cli.go:300-438`); **new CLI respondent-action gap**, **S2 ref gap** |
| Chart | Chart ID in desk JSON (`examples/desk/chart/main.go:227-236`); app-local URL, no kind ref | No chart resolver (`internal/agentos/refs.go:42-62`) | Page create/share/revoke/join with owner guard (`examples/desk/chart/main.go:191-210,360-406`); source polling restricted to loopback (`examples/desk/chart/README.md:94-102`) | SVG chart; source file can use `git` (`examples/desk/chart/README.md:1-4,94-112`) | Checkout register exists (`examples/desk/chart/register.sh:10-24`), host command absent; CLI `add/open/show/set/list/share/close/revoke` but no `join` (`examples/desk/chart/cli.go:112-121`): **new CLI join gap**, **S2 ref gap** |
| Recording | Single take at caller-selected path, no entity ref (`examples/record/record.bsh:14-30,127`) | No media kind (`internal/agentos/refs.go:42-62`) | Local one-take camera page; remote session refused (`examples/record/record.bsh:46-51`) | Mode-0600 photo/audio/video file; explicit file can use `git` or LFS (`examples/record/README.md:44-47`) | `record` registered on host; page is the necessary human capture step and CLI returns path (`examples/record/README.md:18-22`); **S4 file/Git journey to verify**, otherwise verified OK |
| Transcription | File/URL input and output path, no entity ref (`internal/agentos/transcribe.go:28-75`) | No transcript kind (`internal/agentos/refs.go:42-62`) | No separate web app established; caller supplies source | Adapter pipeline/export (`examples/transcribe/adapters/register.sh`; `skills/bashy/commands/transcribe.md`); output file can use `git` | Built-in `transcribe` available per `bashy commands transcribe --features`; **S4 file/Git journey to verify**, otherwise verified OK |
| Video | Project directory/storyboard and `build/demo.mp4`, no entity ref (`examples/video/video.sh:32,385-418`) | No video kind (`internal/agentos/refs.go:42-62`) | No web app; CLI-only workflow | MP4 file; project/output can use `git`/LFS (`examples/video/README.md:18-31`) | `video.sh`/`bashy dag examples/video` exists; no registered `video` command (`bashy commands video --features`: not-found). CLI-only, so **verified OK** for one-way parity; **S4 file/Git journey to verify** |

`examples/google` is an existing **Calendar via `gws`** capability, not a
shared Bashy Calendar web app. Its Bash# registration is in
`examples/google/register.sh:5-15`; `google.bsh:1025-1135` implements
calendar list/get/create/update/delete/freebusy with per-account credential
preflight (`examples/google/README.md:1-15`). It is not installed on this host
(`bashy commands google --features`: not-found). S8 should assess the human web
surface and full user/admin CLI parity; S7 does the same for Memos/Notes.

**Shaping hook:** compose existing `bashy <app> --json`, `bashy kb`, `bashy
define`, `bashy git` and exporter verbs in a reusable Bash# `.bsh` command or
skill wrapper; keep each base verb usable alone. Desk apps expose a `*.bsh`
body, registered command, per-app server and item/template configuration
(`examples/desk/kit/README.md:12-30`); Tutor formats and Viewer templates are
concrete variable behavior (`examples/tutor/README.md:89-105`,
`examples/desk/viewer/templates.go:4-54`). The proposed cross-tool wrapper's
hook is **ref routing by a configurable kind→existing open/export verb map**;
the audit does not implement it. Follow the KB guideline
`extend-bashy-small-verbs-bash-skill-scripts-shaping-hooks-and-th`.

### Proven gaps

| Gap and evidence | Feeds |
|---|---|
| App-local IDs/URLs for Tutor sessions, Klatch, Desk and media have no `define` resolver or uniform emitted `kind:id`; `internal/agentos/refs.go:42-62` versus `docs/command-atlas.md:480-526`. Keep existing local IDs; add only useful cross-tool adapters. | S2 (media file proof also S4) |
| KB has its own resolver/CLI but no Klatch or Viewer browser catalog bridge: `internal/agentos/refs.go:47`, `examples/desk/viewer/main.go:1-15`, `examples/klatch/register.sh:6-14`. | S3 |
| Whiteboard page draw/clear/lock/invite revoke and JSON/PNG download exceed CLI desk seven: `examples/whiteboard/server/index.html:115-120`, `examples/whiteboard/server/cli.go:63-106`. | new (CLI parity; S6 must verify) |
| Brainstorm page idea edit/delete/vote/invite revoke and JSON/Markdown download exceed CLI desk seven: `examples/brainstorm/server/index.html:147-159,182-188`, `examples/brainstorm/server/cli.go:110-130`. | new (CLI parity; S6 must verify) |
| Form respondent `answer` is an HTTP/page action, while CLI `join` only prints a URL and has no `answer` case: `examples/desk/form/server/main.go:345-367`, `examples/desk/form/server/cli.go:432-440`. | new (CLI parity; S6 must verify) |
| Chart HTTP `join` lacks a CLI case: `examples/desk/chart/main.go:191-210`, `examples/desk/chart/cli.go:112-121`. | new (CLI parity; S6 must verify) |
| App/media export-to-Git, explicit remote publish and LFS round trips are not covered by these app commands; file export exists, e.g. `examples/desk/notepad/cli.go:431`, `examples/video/video.sh:418`. Verify the existing `git`/LFS composition before adding code. | S4 |

No gap is inferred solely from a locally missing registration: Chart and Google
have checkout registration scripts, while Video intentionally has only a script.

## Sprint 614 S11: desk page actions

| Page action | CLI verb |
|---|---|
| Whiteboard draw | `bashy whiteboard draw BOARD --points JSON --json` |
| Whiteboard clear | `bashy whiteboard clear BOARD --key KEY --json` |
| Whiteboard lock/unlock | `bashy whiteboard lock BOARD --key KEY [--locked=false] --json` |
| Whiteboard revoke member/invite | `bashy whiteboard revoke BOARD --key KEY --target ID --json` |
| Whiteboard JSON/PNG download | `bashy whiteboard download BOARD --format json\|png [--output FILE] --json` |
| Brainstorm idea edit | `bashy brainstorm edit SESSION --card ID --body TEXT --participant TOKEN --json` |
| Brainstorm idea delete | `bashy brainstorm delete SESSION --card ID --participant TOKEN --json` |
| Brainstorm vote/undo | `bashy brainstorm vote SESSION --card ID --participant TOKEN --json` |
| Brainstorm revoke member/invite | `bashy brainstorm revoke SESSION --target ID --key KEY --json` |
| Brainstorm JSON/Markdown download | `bashy brainstorm download SESSION --format json\|markdown --key KEY [--output FILE] --json` |
| Form respondent answer | `bashy form answer ID --answer FIELD=VALUE [--complete] --json` |
| Chart join | `bashy chart join ID --invite CODE --participant TOKEN --json` |

All write verbs above accept `--dry-run`. `--participant` carries the same member identity across separate CLI invocations.
