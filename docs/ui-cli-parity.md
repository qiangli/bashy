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
| Members page: circle field | `bashy member invite EMAIL --circle NAME` / `bashy member set EMAIL --circle NAME` — any owner-chosen name (lowercase slug, up to 32 characters; `public` and `owner` reserved) |
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
| Apps page: share an app with a circle | `bashy outpost apps share APP --circle C` — circle membership is checked live |
| Suites page: list, create, add, remove, delete | `bashy suite ls` / `set NAME HOST/APP...` / `add NAME HOST/APP` / `remove NAME HOST/APP` / `rm NAME` (`show NAME` reads one suite) |
| Suites page: share, unshare, list grants | `bashy suite share NAME --circle C` (or `--email E`) / `unshare NAME SHARE_ID` / `shares NAME` — circle membership is checked live |

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
| Whiteboard | Named board in saved JSON (`examples/whiteboard/README.md:1-32`); local name/URL, no kind ref | No whiteboard resolver (`internal/agentos/refs.go:42-62`) | Page draws, locks, shares via collab server; Excalidraw fork (`examples/whiteboard/server/desk.go:81-296`; `server/storage.go:32-90`) | Browser JSON/PNG/SVG export; backup/restore, files can use `git` | Fork wrapper `examples/whiteboard/whiteboard.bsh:16-21` is lifecycle-only (`install/start/stop/status/source`; former `server/cli.go` removed in Sprint 631 S5); **Sprint 632 A1 GAP owned by A5** |
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
| **Addressed by S2:** registered apps with `show <id> --json` use the generic `app:<command>/<id>` ref, for example `bashy define app:notepad/<id>`. The app's own access check remains authoritative. Media file proof is tracked separately. | S2 (media file proof also S4) |
| KB has its own resolver/CLI but no Klatch or Viewer browser catalog bridge: `internal/agentos/refs.go:47`, `examples/desk/viewer/main.go:1-15`, `examples/klatch/register.sh:6-14`. | S3 |
| Whiteboard page actions (boards list/new/show/lock, scene save/load, files, export): `examples/whiteboard/server/cli.go` was removed when Whiteboard was re-wrapped as an Excalidraw fork in Sprint 631 S5; current `whiteboard.bsh:16-21` is lifecycle-only. | **GAP owned by A5 (`876cd327`)** (refreshed in Sprint 632 A1 below) |
| Brainstorm page idea edit/delete/vote/invite revoke and JSON/Markdown download exceed CLI desk seven: `examples/brainstorm/server/index.html:147-159,182-188`, `examples/brainstorm/server/cli.go:110-130`. | new (CLI parity; S6 must verify) |
| Form respondent `answer` is an HTTP/page action, while CLI `join` only prints a URL and has no `answer` case: `examples/desk/form/server/main.go:345-367`, `examples/desk/form/server/cli.go:432-440`. | new (CLI parity; S6 must verify) |
| Chart HTTP `join` lacks a CLI case: `examples/desk/chart/main.go:191-210`, `examples/desk/chart/cli.go:112-121`. | new (CLI parity; S6 must verify) |
| App/media export-to-Git, explicit remote publish and LFS round trips are not covered by these app commands; file export exists, e.g. `examples/desk/notepad/cli.go:431`, `examples/video/video.sh:418`. Verify the existing `git`/LFS composition before adding code. | S4 |

No gap is inferred solely from a locally missing registration: Chart and Google
have checkout registration scripts, while Video intentionally has only a script.

## Sprint 614 S11: desk page actions

> [!NOTE]
> Whiteboard note (refreshed Sprint 632 A1): The whiteboard rows below originally cited `examples/whiteboard/server/cli.go`, which no longer exists. Whiteboard was re-wrapped in Sprint 631 S5 as an Excalidraw fork (`qiangli/excalidraw-shape`), whose current custom command (`whiteboard.bsh:16-21`) exposes lifecycle only (`install/start/stop/status/source`). The server-backed verbs below were tracked as GAPs owned by story A5 (`876cd327`) and are now implemented; the A1 matrix and gap-ownership rows stay as the audit record. Page-only canvas actions record their reason instead of filing a gap.

| Page action | CLI verb |
|---|---|
| Whiteboard boards list | `bashy whiteboard boards --json` (A5 `876cd327`) |
| Whiteboard board create/open | `bashy whiteboard new [NAME] [--note TEXT] --json` / `bashy whiteboard show ID --json` (A5 `876cd327`) |
| Whiteboard board note | `bashy whiteboard set ID --note TEXT --json` (A5 `876cd327`) |
| Whiteboard draw | Page-only reason: canvas drawing is client-side only (no server REST API) |
| Whiteboard shape/arrow/text, move/resize/rotate | Page-only reason: client-side canvas state, ephemeral Socket.IO diffs |
| Whiteboard clear | Page-only reason: client-side canvas reset |
| Whiteboard zoom/pan/grid/theme | Page-only reason: client-side display preference; no server state |
| Whiteboard lock | `bashy whiteboard lock ID --json` (one-way close; the page has no unlock) (A5 `876cd327`) |
| Whiteboard remove | `bashy whiteboard rm ID --json` (missing board is `not_found`, as on the page) (A5 `876cd327`) |
| Whiteboard share/collab link | `bashy whiteboard share ID --json` (read-only; prints the `#room=` link) (A5 `876cd327`) |
| Whiteboard revoke member/invite | Page-only reason: Excalidraw room sharing uses room IDs; the service keeps no member list |
| Whiteboard scene save/load | `bashy whiteboard save --file PATH --json` / `bashy whiteboard load SCENE-ID [--output PATH]` (A5 `876cd327`) |
| Whiteboard asset upload/download | `bashy whiteboard upload PATH --file LOCAL` / `bashy whiteboard download PATH [--output LOCAL]` (A5 `876cd327`) |
| Whiteboard board JSON export | `bashy whiteboard export ID [--scene SCENE-ID] [--output PATH] --json` (A5 `876cd327`) |
| Whiteboard PNG/SVG export | Page-only reason: the browser renders the canvas; no server API exists (`export --format png\|svg` is refused) |
| Brainstorm idea edit | `bashy brainstorm edit SESSION --card ID --body TEXT --participant TOKEN --json` |
| Brainstorm idea delete | `bashy brainstorm delete SESSION --card ID --participant TOKEN --json` |
| Brainstorm vote/undo | `bashy brainstorm vote SESSION --card ID --participant TOKEN --json` |
| Brainstorm revoke member/invite | `bashy brainstorm revoke SESSION --target ID --key KEY --json` |
| Brainstorm JSON/Markdown download | `bashy brainstorm download SESSION --format json\|markdown --key KEY [--output FILE] --json` |
| Form respondent answer | `bashy form answer ID --answer FIELD=VALUE [--complete] --json` |
| Chart join | `bashy chart join ID --invite CODE --participant TOKEN --json` |

All write verbs above accept `--dry-run`. `--participant` carries the same member identity across separate CLI invocations.

## Sprint 632 A1: Shapeable apps parity audit (eight catalog apps)

Audit of web/page/HTTP actions against bashy CLI companion verbs for the eight catalog apps delivered in Sprint 631 and shaped for Sprint 632.

Parity is **one-way**: every web, page, and HTTP action of a catalog app must have a companion bashy verb (`--json`; `--dry-run` on writes; same owner/member auth checks and the same denied result as the page). CLI-only features need no page.

### Current state verified by review (commands run)
1. `notes`, `board`, `whiteboard`, `table`: expose only lifecycle verbs (`install|start|stop|status|source`) via their `.bsh` command wrappers; all application data and management actions are GAPs.
   - Verification command: `bashy/examples/<app>/<app>.bsh --help` -> only `install|start|stop|status|source` printed.
2. `form`: exposes action verbs (`add`, `reply`, `ask`, `open`, `show`, `set`, `list`, `share`, `close`, `wait`, `join`, `answer`).
   - Verification command: `bashy --bashsharp bashy/examples/desk/form/form.bsh --help` -> advertises `append` but not `join` or `answer`; `form.bsh:97` case statement handles `join` but omits `answer` (which falls through to `*)`); `server/cli.go:273,466` implements both `answer` and `join`. Owned by A6 (`e736fdb1`).
3. `clips`: exposes action verbs (`queue`, `status`, `work`, `take`, `serve`, `doctor`). 100% parity across all web/HTTP actions.
   - Verification command: `bashy --bashsharp bashy/examples/clip/clip.bsh --help`.
4. `site` (+ chat template): static company site exposes `new`, `build`, `serve`, `edit`, `publish-outpost`, `publish-pages`, `shape-register`. Chat template (`site.bsh chat` / `site new --template chat`) exposes `new`, `start`, `stop`, `status`, `source`; web chat prompts and project export are GAPs owned by A6 (`e736fdb1`).
   - Verification command: `bashy --bashsharp bashy/examples/site/site.bsh --help` and `bashy --bashsharp bashy/examples/site/chat/chat.bsh --help`.
5. `files`: outpost built-in File Browser (embedding `qiangli/filebrowser-quantum`, Apache-2.0, in `outpost/cmd/outpost/capability_files.go`) is reached through `bashy outpost connect`, `bashy outpost ssh`, and `bashy outpost scp` (`bashy/docs/ui-cli-parity.md:87`). All file browsing, transfer, and edit actions are covered.

### Parity matrix

| App | Action / HTTP Endpoint | Existing verb or GAP | Evidence (file:line or command output) |
|---|---|---|---|
| **notes** | Install pinned source build | `bashy notes install` | `examples/notes/notes.bsh:37-81` |
| **notes** | Start console and app server | `bashy notes start` | `examples/notes/notes.bsh:96-106` |
| **notes** | Stop app console | `bashy notes stop` | `examples/notes/notes.bsh:107-115` |
| **notes** | Check status and revision pin | `bashy notes status` | `examples/notes/notes.bsh:116-119` |
| **notes** | Print managed source directory | `bashy notes source` | `examples/notes/notes.bsh:120` |
| **notes** | List memos timeline (`GET /notes/api/v1/memos`) | **GAP (A2 `57624adf`)** | `examples/notes/notes.bsh:16-21`, `web-prefix.patch:connect.ts:66` |
| **notes** | View single memo (`GET /notes/api/v1/memos/{id}`) | **GAP (A2 `57624adf`)** | `examples/notes/notes.bsh:16-21` |
| **notes** | Create/post memo (`POST /notes/api/v1/memos`) | **GAP (A2 `57624adf`)** | `examples/notes/notes.bsh:16-21` |
| **notes** | Edit/update memo (`PATCH /notes/api/v1/memos/{id}`) | **GAP (A2 `57624adf`)** | `examples/notes/notes.bsh:16-21` |
| **notes** | Archive memo (`PATCH /notes/api/v1/memos/{id}` state=ARCHIVED) | **GAP (A2 `57624adf`)** | `examples/notes/notes.bsh:16-21` |
| **notes** | Unarchive memo (`PATCH /notes/api/v1/memos/{id}` state=NORMAL) | **GAP (A2 `57624adf`)** | `examples/notes/notes.bsh:16-21` |
| **notes** | Delete memo (`DELETE /notes/api/v1/memos/{id}`) | **GAP (A2 `57624adf`)** | `examples/notes/notes.bsh:16-21` |
| **notes** | Search/filter memos by text/tag (`GET /notes/api/v1/memos:search`) | **GAP (A2 `57624adf`)** | `examples/notes/notes.bsh:16-21` |
| **notes** | Export memos to Markdown/JSON (`GET /notes/api/v1/memos:export`) | **GAP (A2 `57624adf`)** | `examples/notes/notes.bsh:16-21` |
| **notes** | List tags (`GET /notes/api/v1/tags`) | **GAP (A2 `57624adf`)** | `examples/notes/notes.bsh:16-21` |
| **notes** | Upload attachment/resource (`POST /notes/api/v1/resources`) | **GAP (A2 `57624adf`)** | `examples/notes/notes.bsh:16-21` |
| **notes** | Live update stream (`GET /notes/api/sse`) | Server stream transport; companion verb `notes list` (A2) | `examples/notes/web-prefix.patch:useLiveMemoRefresh.ts:402` |
| **form** | View form loopback page (`GET /`) | Browser step / `bashy form open ID` (prints URL) | `examples/desk/form/server/main.go:302-306`, `server/cli.go:369-377` |
| **form** | Health check (`GET /healthz`) | `curl http://127.0.0.1:18107/healthz` | `examples/desk/form/server/main.go:308-311` |
| **form** | Form state query (`GET /api/state?id=ID`) | `bashy form show ID --json` | `examples/desk/form/server/main.go:312-328`, `server/cli.go:379-388` |
| **form** | Render Markdown preview (`POST /api/render`) | Internal render helper / `bashy form reply` | `examples/desk/form/server/main.go:329-332`, `server/cli.go:291-311` |
| **form** | Define/create form (`POST /api/action` action="add") | `bashy form add --def FILE [--title TITLE] --json` | `examples/desk/form/server/main.go:345-346`, `server/cli.go:334-367` |
| **form** | Prefill/reply assist form (`POST /api/action` action="reply") | `bashy form reply --extract FILE --values FILE --refusals FILE --json` | `examples/desk/form/server/main.go:347-348`, `server/cli.go:291-311` |
| **form** | Create questionnaire (`POST /api/action` action="ask") | `bashy form ask --def FILE [--title TITLE] --json` | `examples/desk/form/server/main.go:349-350`, `server/cli.go:313-332` |
| **form** | Surface form URL (`POST /api/action` action="open") | `bashy form open ID --json` | `examples/desk/form/server/main.go:351-352`, `server/cli.go:369-377` |
| **form** | Read form and answers (`POST /api/action` action="show") | `bashy form show ID --json` | `examples/desk/form/server/main.go:353-354`, `server/cli.go:379-388` |
| **form** | Mutate/prefill answers (`POST /api/action` action="set") | `bashy form set ID --answer FIELD=VALUE --json` | `examples/desk/form/server/main.go:355-356`, `server/cli.go:414-438` |
| **form** | Close form (`POST /api/action` action="close") | `bashy form close ID --json` | `examples/desk/form/server/main.go:357-358`, `server/cli.go:456-464` |
| **form** | Submit respondent answer (`POST /api/action` action="answer") | `bashy form answer ID --answer FIELD=VALUE [--complete] --json` | `examples/desk/form/server/main.go:359-360`, `server/cli.go:273-289` |
| **form** | List forms (`POST /api/action` action="list") | `bashy form list --json` | `examples/desk/form/server/main.go:361-362`, `server/cli.go:440-444` |
| **form** | Share form with members (`POST /api/action` action="share") | `bashy form share ID --json` | `examples/desk/form/server/main.go:363-364`, `server/cli.go:446-454` |
| **form** | Join form with invite code (`POST /api/action` action="join") | `bashy form join ID --invite CODE --json` | `examples/desk/form/server/main.go:365-366`, `server/cli.go:466-474` |
| **form** | Wait for answers | `bashy form wait ID [--timeout DURATION] --json` | `examples/desk/form/server/cli.go:389-412` |
| **form** | Foreground service lifecycle | `bashy form serve` / `bashy form stop` | `examples/desk/form/form.bsh:83-96` |
| **form** | CLI help text / case statement drift | **GAP (A6 `e736fdb1`)** — `--help` advertises `append` instead of `join`/`answer`; case handles `join` but omits `answer` | `examples/desk/form/form.bsh:81,97`, `server/cli.go:273,466` |
| **clips** | View library page (`GET /`) | Browser step / `bashy clips serve` | `examples/clip/server/page.go:58-70`, `clip.bsh:84-93` |
| **clips** | Service health check (`GET /healthz`) | `bashy clips doctor` | `examples/clip/server/page.go:20-23`, `clip.bsh:80` |
| **clips** | List jobs and status (`GET /api/jobs`) | `bashy clips status [ID] [--json]` | `examples/clip/server/page.go:24-32`, `server/main.go:222-243` |
| **clips** | Queue video segment (`POST /api/jobs` kind="video") | `bashy clips queue --from VIDEO.mp4 --start S --end E [--name NAME]` | `examples/clip/server/page.go:33-44`, `server/main.go:121-196` |
| **clips** | Queue asciicast render (`POST /api/jobs` kind="cast") | `bashy clips queue --from SCENE.cast [--start S --end E] [--format gif\|mp4]` | `examples/clip/server/page.go:33-44`, `server/main.go:121-196` |
| **clips** | Queue screen recording (`POST /api/jobs` kind="screen") | `bashy clips queue screen --seconds S [--display :0.0] [--size WxH]` | `examples/clip/server/page.go:33-44`, `server/main.go:121-196` |
| **clips** | Stream/play clip artifact (`GET /media/{name}`) | `bashy clips take ID` (outputs finished artifact path) | `examples/clip/server/page.go:50-57`, `server/main.go:245-274` |
| **clips** | Render queued jobs in background | `bashy clips work [--once] [--verbose]` | `examples/clip/server/worker.go`, `clip.bsh:80` |
| **clips** | Start library server | `bashy clips serve` / `bashy clips stop` | `examples/clip/clip.bsh:84-96` |
| **site** | Initialize new company site | `bashy site new [DIR] [--title TITLE]` | `examples/site/site.bsh:46-90` |
| **site** | Generate static HTML | `bashy site build [DIR]` | `examples/site/site.bsh:92-119` |
| **site** | Serve static site on console | `bashy site serve [DIR] [NAME]` | `examples/site/site.bsh:121-142` |
| **site** | Open Sveltia CMS & visual editor | `bashy site edit [NAME]` | `examples/site/site.bsh:144-148` |
| **site** | Publish to Outpost | `bashy site publish-outpost NAME` | `examples/site/site.bsh:150-156` |
| **site** | Publish to GitHub Pages | `bashy site publish-pages [DIR] REPO [BRANCH]` | `examples/site/site.bsh:158-184` |
| **site** | Register as shapeable app | `bashy site shape-register [DIR] [NAME]` | `examples/site/site.bsh:186-193` |
| **site** | Content authoring in Sveltia CMS | Direct markdown/file editing or Git commit | `examples/site/site.bsh:20-28` |
| **site (chat)** | Install chat site from fork | `bashy site new --template chat [NAME] [--port PORT]` | `examples/site/chat/chat.bsh:53-93` |
| **site (chat)** | Start chat app service | `bashy site start [NAME]` | `examples/site/chat/chat.bsh:107-126` |
| **site (chat)** | Stop chat app service | `bashy site stop [NAME]` | `examples/site/chat/chat.bsh:128-139` |
| **site (chat)** | Status of chat app service | `bashy site status [NAME]` | `examples/site/chat/chat.bsh:141-155` |
| **site (chat)** | Print chat source directory | `bashy site source [NAME]` | `examples/site/chat/chat.bsh:157-160` |
| **site (chat)** | Web chat prompt to generate/edit site | **GAP (A6 `e736fdb1`)** — no companion CLI prompt verb | `examples/site/chat/chat.bsh:16-29` |
| **site (chat)** | Export generated project bundle | **GAP (A6 `e736fdb1`)** — no companion CLI export verb | `examples/site/chat/chat.bsh:16-29` |
| **board** | Install pinned source build | `bashy board install` | `examples/board/board.bsh:38-111` |
| **board** | Start console and API/Postgres | `bashy board start` | `examples/board/board.bsh:126-136` |
| **board** | Stop app console | `bashy board stop` | `examples/board/board.bsh:137-145` |
| **board** | Check status and revision pin | `bashy board status` | `examples/board/board.bsh:146-149` |
| **board** | Print managed source directory | `bashy board source` | `examples/board/board.bsh:150` |
| **board** | List projects (`GET /board/api/projects`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21`, `board-console/main.go:80-87` |
| **board** | Create project (`POST /board/api/projects`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | Show project details (`GET /board/api/projects/{id}`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | Update project (`PATCH /board/api/projects/{id}`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | Delete project (`DELETE /board/api/projects/{id}`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | List tasks in project (`GET /board/api/projects/{id}/tasks`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | Add task / card (`POST /board/api/tasks`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | Show task details (`GET /board/api/tasks/{id}`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | Set / update task (`PATCH /board/api/tasks/{id}`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | Move task column/status (`PATCH /board/api/tasks/{id}/move`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | Delete task (`DELETE /board/api/tasks/{id}`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | List columns (`GET /board/api/projects/{id}/columns`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | Add column (`POST /board/api/projects/{id}/columns`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | List comments on task (`GET /board/api/tasks/{id}/comments`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | Add comment to task (`POST /board/api/tasks/{id}/comments`) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **board** | Export board tasks (JSON/CSV) | **GAP (A3 `c2f2f915`)** | `examples/board/board.bsh:16-21` |
| **whiteboard** | Install pinned source build | `bashy whiteboard install` | `examples/whiteboard/whiteboard.bsh:40-90` |
| **whiteboard** | Start console and server | `bashy whiteboard start` | `examples/whiteboard/whiteboard.bsh:105-120` |
| **whiteboard** | Stop app console | `bashy whiteboard stop` | `examples/whiteboard/whiteboard.bsh:121-129` |
| **whiteboard** | Check status and revision pin | `bashy whiteboard status` | `examples/whiteboard/whiteboard.bsh:130-133` |
| **whiteboard** | Print managed source directory | `bashy whiteboard source` | `examples/whiteboard/whiteboard.bsh:134-137` |
| **whiteboard** | List boards (`GET /whiteboard/api/board` or action="list") | **GAP (A5 `876cd327`)** | `examples/whiteboard/server/desk.go:81-119` |
| **whiteboard** | Create/add board (`POST /whiteboard/api/action` action="add") | **GAP (A5 `876cd327`)** | `examples/whiteboard/server/desk.go:188-217` |
| **whiteboard** | Show board state (`GET /whiteboard/api/state?id=ID`) | **GAP (A5 `876cd327`)** | `examples/whiteboard/server/desk.go:121-154,218-240` |
| **whiteboard** | Set board note (`POST /whiteboard/api/action` action="set") | **GAP (A5 `876cd327`)** | `examples/whiteboard/server/desk.go:241-263` |
| **whiteboard** | Close/lock board (`POST /whiteboard/api/action` action="close") | **GAP (A5 `876cd327`)** | `examples/whiteboard/server/desk.go:264-281` |
| **whiteboard** | Share room link (`POST /whiteboard/api/action` action="share") | **GAP (A5 `876cd327`)** | `examples/whiteboard/server/desk.go:285-296` |
| **whiteboard** | Save scene buffer (`POST /whiteboard/api/v2/post`) | **GAP (A5 `876cd327`)** | `examples/whiteboard/server/storage.go:55-73` |
| **whiteboard** | Load scene buffer (`GET /whiteboard/api/v2/{id}`) | **GAP (A5 `876cd327`)** | `examples/whiteboard/server/storage.go:75-89` |
| **whiteboard** | Upload asset file (`POST /whiteboard/files/{path}`) | **GAP (A5 `876cd327`)** | `examples/whiteboard/server/storage.go:91-100` |
| **whiteboard** | Download asset file (`GET /whiteboard/files/{path}`) | **GAP (A5 `876cd327`)** | `examples/whiteboard/server/storage.go:91-100` |
| **whiteboard** | Export board to PNG/SVG/JSON | **GAP (A5 `876cd327`)** | `examples/whiteboard/whiteboard.bsh:16-21` |
| **whiteboard** | Freehand canvas drawing | Client-side React state, ephemeral Socket.IO diffs; no server REST API | `examples/whiteboard/server/collab.go:50-70` |
| **whiteboard** | Insert shape/arrow/text tool | Client-side React state; no server REST API | `examples/whiteboard/server/collab.go` |
| **whiteboard** | Move/resize/rotate element | Client-side React state; no server REST API | In-browser canvas event |
| **whiteboard** | Clear canvas | Client-side canvas reset; no server REST API | In-browser canvas event |
| **whiteboard** | Zoom, pan, grid, theme toggle | Client-side user display preference; no server state | In-browser canvas event |
| **table** | Install pinned source build | `bashy table install` | `examples/table/table.bsh:47-102` |
| **table** | Start console and Grist runtime | `bashy table start` | `examples/table/table.bsh:117-133` |
| **table** | Stop app console | `bashy table stop` | `examples/table/table.bsh:134-142` |
| **table** | Check status and revision pin | `bashy table status` | `examples/table/table.bsh:143-146` |
| **table** | Print managed source directory | `bashy table source` | `examples/table/table.bsh:147` |
| **table** | List documents (`GET /table/api/docs`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | Create document (`POST /table/api/docs`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | Show document details (`GET /table/api/docs/{docId}`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | Delete document (`DELETE /table/api/docs/{docId}`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | List tables in document (`GET /table/api/docs/{docId}/tables`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | Create table (`POST /table/api/docs/{docId}/tables`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | List columns (`GET /table/api/docs/{docId}/tables/{tableId}/columns`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | Add column (`POST /table/api/docs/{docId}/tables/{tableId}/columns`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | List records (`GET /table/api/docs/{docId}/tables/{tableId}/records`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | Add records (`POST /table/api/docs/{docId}/tables/{tableId}/records`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | Set / update records (`PATCH /table/api/docs/{docId}/tables/{tableId}/records`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | Delete records (`POST /table/api/docs/{docId}/tables/{tableId}/data/delete`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | List views and page layouts | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | Export table to CSV (`GET /table/api/docs/{docId}/download/csv`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | Export / download full document | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **table** | Import CSV to table (`POST /table/api/docs/{docId}/tables/{tableId}/data`) | **GAP (A4 `c49679cb`)** | `examples/table/table.bsh:17-22` |
| **files** | Reach built-in File Browser | `bashy outpost connect`, `bashy outpost ssh\|scp` | `bashy/docs/ui-cli-parity.md:87`, `outpost/cmd/outpost/capability_files.go:16-105` |
| **files** | Authenticate / login (`POST /api/login`) | `bashy outpost register` / pair credential / local tunnel auth | `outpost/cmd/outpost/capability_files.go:34-45` |
| **files** | Browse directory (`GET /api/resources/{path}`) | `bashy outpost ssh "ls -la {path}"` / `bashy outpost connect` | `bashy/docs/ui-cli-parity.md:87` |
| **files** | Read / download file (`GET /api/raw/{path}`) | `bashy outpost scp HOST:{path} LOCAL` or `bashy outpost ssh "cat {path}"` | `bashy/docs/ui-cli-parity.md:87` |
| **files** | Upload file (`POST /api/resources/{path}`) | `bashy outpost scp LOCAL HOST:{path}` (requires `allow_write`) | `outpost/cmd/outpost/capability_files.go:27-30` |
| **files** | Create directory (`POST /api/resources/{path}/`) | `bashy outpost ssh "mkdir -p {path}"` (requires `allow_write`) | `outpost/cmd/outpost/capability_files.go:27-30` |
| **files** | Edit / save text file (`PUT /api/resources/{path}`) | `bashy outpost scp` / remote editor via `bashy outpost ssh` / `connect` | `outpost/cmd/outpost/capability_files.go:27-30` |
| **files** | Rename / move file or directory (`PATCH /api/resources/{path}`) | `bashy outpost ssh "mv {old} {new}"` (requires `allow_write`) | `outpost/cmd/outpost/capability_files.go:27-30` |
| **files** | Delete file or directory (`DELETE /api/resources/{path}`) | `bashy outpost ssh "rm -rf {path}"` (requires `allow_write`) | `outpost/cmd/outpost/capability_files.go:27-30` |
| **files** | Preview media, image, or document | Companion CLI inspection via `bashy lens`, `cat`, `ffprobe`, `outpost scp` | Read-only inspect, covered by CLI |

### Gap ownership & phase-1 story handoff

Every GAP identified in the audit is strictly owned by an active phase-1 story in Sprint 632:

1. **A2 notes (`57624adf88eb`):** owns all 10 notes data and management gaps (`list`, `show`, `add`, `set`, `archive`, `unarchive`, `rm`, `search`, `export`, `tags`, `upload`).
2. **A3 board (`c2f2f915c54d`):** owns all 14 board gaps (`projects list/add/show/set/rm`, `tasks list/add/show/set/move/rm`, `columns list/add`, `comments add/list`, `export`).
3. **A4 table (`c49679cb7173`):** owns all 14 table gaps (`docs list/add/show/rm`, `tables list/add`, `columns list/add`, `records list/add/set/rm`, `views list`, `export csv/doc`, `import csv`).
4. **A5 whiteboard (`876cd327a5e6`):** owns all 10 server-backed whiteboard gaps (`boards list/add/show/set/lock/share`, `scene save/load`, `files upload/download`, `export`). Page-only canvas drawing actions are documented with reasons and require no server CLI verb.
5. **A6 form/clips/site (`e736fdb12e9e`):** owns form `--help` drift (`append` vs `join`/`answer`), and site chat template prompt and code export gaps.
6. **files:** fully covered via `bashy outpost connect/ssh/scp`; zero unowned gaps.

Because all gaps are accounted for in stories A2–A6, no orphan gap exists and no standalone `--no-sprint` todo is filed.

## Sprint 632 A2: notes data command mappings

Source: `examples/notes/notes.bsh`, `notes_api.py`, pinned fork
`d0c6e2a7afc6de17220f9b439bb077a5aa12f83f` (`proto/api/memo_service.proto`).
All listed data verbs accept `--json`; every write accepts `--dry-run`.
They use the existing principal's app token on the running local instance.
App response/error objects pass through unchanged. Local errors use
`{code,message}`; exit 2 means invalid arguments, exit 1 means API/transport
failure. Dry-run is `{dryRun,method,url,body}` and performs no I/O. These are
unit-verified mappings; running-fork authorization proof is still open.

| Page action | Verb or recorded reason |
|---|---|
| Browse notes / next page / sort / filter | `notes list --page-size N --page-token TOKEN --order-by ORDER --filter CEL --json` |
| Read note | `notes show ID --json` |
| Create note | `notes add --content MARKDOWN --visibility private --json` |
| Edit content / task checkboxes | `notes set ID --content MARKDOWN --json` (full replacement, including task markers) |
| Edit tags | `notes set ID --content MARKDOWN --tags a,b --json`; tags are derived from content, so removing/changing inline hashtags is a content edit |
| Change visibility | `notes set ID --visibility private\|protected\|public\|space --json` |
| Pin / unpin | `notes set ID --pinned true\|false --json` |
| Move to a space / remove placement | `notes set ID --space spaces/ID --json` / `--space ''` |
| Archive / restore | `notes archive ID --json` / `notes unarchive ID --json` |
| Browse archived notes | `notes list --state archived --json` |
| Delete note and its comment descendants | `notes rm ID --json` |
| Search notes | `notes search TEXT --filter CEL --json` |
| Export one note's Markdown / JSON | `notes export ID --format markdown\|json`; `--json` always returns the app Memo object |
| Full settings backup ZIP / import | Deferred: separate streamed file and staged import APIs; single-note export does not claim backup parity |
| Attachments, relations, comments, reactions, share-link create/revoke | Deferred: separate subresources in the pinned API, outside A2's requested memo data verbs; content edits do not claim these actions |
| Account, space membership, notification and instance settings | Deferred: account/administration API scope, outside A2's memo data verbs; no new identity or settings store introduced |
| Shape wish / verify / preview / approve / undo | Existing `bashy shape` lifecycle, as documented in `examples/notes/README.md` |

A1 owns the catalog audit section; these rows provide the A2 implementation
mapping and explicit remaining scope for reconciliation with that audit.

### Table — A4 data command mappings

Source: `examples/table/table.bsh`, `table_api.py`, pinned fork
`a3c557ac1bc4bd55b9b9d4333842bec255586ff4` (Grist REST API). All data verbs
accept `--json`; every write accepts `--dry-run`. They call the running local
instance's REST API with the principal's existing API key (`TABLE_URL`,
`TABLE_TOKEN`), never the database. App responses and denials pass through
unchanged; local errors use `{code,message}` (exit 2 invalid arguments, exit 1
API/transport). Unit-verified mappings; running-fork authorization proof is
still open.

| Page action | Verb or recorded reason |
|---|---|
| Browse documents / workspaces | `table docs list [--org ORG] --json` |
| Create document | `table docs add NAME --workspace ID --json` |
| Open document details | `table docs show DOC --json` |
| List tables | `table tables list DOC --json` |
| Add table (with columns) | `table tables add DOC TABLE [--columns ID[:Type],...] --json` |
| List columns | `table columns list DOC TABLE --json` |
| Add column | `table columns add DOC TABLE COLUMN [--type T] [--label L] --json` |
| Browse / filter / sort records | `table records list DOC TABLE --where COL=VAL --sort=[-]COL --limit N --json` |
| Add row | `table records add DOC TABLE --set COL=VAL --json` |
| Edit cells | `table records set DOC TABLE ROWID... --set COL=VAL --json` (or `--where`) |
| Delete rows | `table records rm DOC TABLE ROWID... --json` (or `--where`) |
| List pages / views | `table views list DOC --json` (read-only view metadata) |
| Download table as CSV / JSON | `table export DOC TABLE --format csv\|json` |
| Import CSV into a table | `table import DOC TABLE --file PATH.csv --json` (batches of 500, not transactional) |
| Rename / delete / reorder columns, tables, documents; column type change | Deferred: separate schema-action endpoints, outside the requested verbs; no direct database edit is offered |
| Create / edit views, pages, widgets, sorts and filters saved in a view | Deferred: view layout is edited through document user actions, not a stable REST resource; `views list` is read-only |
| Formulas, conditional rules, access rules, sharing and permissions, webhooks | Deferred: administration/ACL scope; the app stays the authority |
| Import from Excel/Google Sheets/JSON, full document download or restore | Deferred: staged upload API; only CSV rows import is claimed |
| Shape wish / verify / preview / approve / undo | Existing `bashy shape` lifecycle, as documented in `examples/table/README.md` |

### Board — A1 audit reconciled with A3 data commands

Source: `examples/board/board.bsh`, `board_api.py`; fork pin
`5069fb56ce309d5572ca3ed60d4d3c322a03a452`,
`apps/api/src/{project,task,column,comment}/{index,schema}.ts`.
These rows reconcile all 16 board data actions in the A1 draft. The pinned
routes use singular `/project`, `/task`, `/column`, `/comment` and `PUT`,
not the draft's inferred plural routes and `PATCH`. Paths below are relative
to `BOARD_URL`, the running principal's numeric loopback API base (including
`/api` or the adapter's `/board/api`). `BOARD_TOKEN` supplies the existing
principal's bearer session/API token; no account or credential is created.

Every data verb accepts `--json`; every write accepts `--dry-run`. A2's
response convention applies: app JSON passes through (including lists and
pagination), local errors are `{code,message}`, and dry-run returns
`{dryRun,method,url,body}` without reading credentials or doing network I/O.
API denials pass through unchanged, including plain-text errors; exit 1
means API/transport failure, exit 2 invalid arguments. Redirects and proxies
are disabled. No client-side owner bypass or alternate database path exists.
Unit coverage is green; running-fork authorization and combined S4b runtime
no-phone-home proof remain open (see `examples/board/docs/s632-a3.md`).

| A1 page action | Verb | Pinned API |
|---|---|---|
| Install / start / stop / status / source | Existing `board install\|start\|stop\|status\|source` | Managed lifecycle; no data request |
| List projects | `board projects list --workspace ID [--include-archived true] --json` | `GET /project/?workspaceId=ID` |
| Add project | `board projects add --workspace ID --name TEXT --icon TEXT --slug KEY --json` | `POST /project/` |
| Show project | `board projects show ID --json` | `GET /project/ID` |
| Edit project | `board projects set ID --name TEXT --icon TEXT --slug KEY --description TEXT --public false --json` | `PUT /project/ID`; all five replacement fields required; optional `--include-subtasks` |
| Delete project | `board projects rm ID --json` | `DELETE /project/ID`; includes its contents, as on the page |
| List tasks | `board tasks list --project ID --page N --limit N --json` | `GET /task/tasks/ID`; API pagination/filters preserved |
| Add task/card | `board tasks add --project ID --title TEXT --description TEXT --priority low --status SLUG --json` | `POST /task/ID`; optional assignee/start/due dates |
| Show task | `board tasks show ID --json` | `GET /task/ID` |
| Edit task | `board tasks set ID --title TEXT --json` (also single-field description, priority, assignee, due-date, status) | `PUT /task/{title,description,priority,assignee,due-date,status}/ID` |
| Edit several task fields / position | `board tasks set ID --title TEXT --priority low --status SLUG --project ID --position N --json` | `PUT /task/ID`; full required replacement fields, optional description/assignee/dates; one atomic request |
| Move task column/status | `board tasks move ID --column SLUG --json` (`--status` alias) | `PUT /task/status/ID`; uses the column slug, not its display name |
| Delete task | `board tasks rm ID --json` | `DELETE /task/ID` |
| List columns | `board columns list --project ID --json` | `GET /column/ID` |
| Add column | `board columns add --project ID --name TEXT --json` | `POST /column/ID`; optional icon/color/final |
| List task comments | `board comments list TASK_ID --json` | `GET /comment/TASK_ID` |
| Add comment | `board comments add TASK_ID --content TEXT --json` | `POST /comment/TASK_ID`; same authenticated author and permission checks as page |
| Export project tasks JSON/CSV | `board export PROJECT_ID --format json\|csv` | `GET /task/export/PROJECT_ID`; CSV serializes the complete export response's tasks, not a paginated task list; `--json` always returns original API JSON |

CSV contains title, description, status, priority, startDate, dueDate, userId,
and labels (JSON in one quoted field). It preserves user content, commas,
quotes and newlines. Export is a project-task snapshot, not a database backup.
Task list additionally accepts status, priority, assignee, related-page,
sort-by, sort-order, due-before and due-after. Empty single-field assignee
and due-date edits clear those fields. The API validates column existence,
workspace membership and privileges; the CLI never infers authority from IDs.

Beyond A1's board rows, column edit/reorder/delete, comment edit/delete,
project archive/reorder/cross-workspace moves, task cross-project moves,
recurrence, labels, relations, attachments/backgrounds, time entries,
workspace/account administration, integrations and imports remain deferred:
they use separate API resources outside this story's requested data verbs.
No parity is claimed for them. Shape actions retain the existing `bashy shape`
lifecycle. Browser layout/theme preferences are client-local UI state.
