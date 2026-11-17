# Google Workspace Custom Command (`examples/google`)

This example exposes Google Workspace services (**Gmail**, **Google Calendar**, **Google Tasks**, and **Google Drive**) inside `bashy` through a self-contained Bash# custom command (`google.bsh`) and embedded skill wrapping the `gws` (`googleworkspace/cli`) binary.

## Files

- `google.bsh` — Executable Bash# script (`#!/usr/bin/env -S bashy --bashsharp`) with embedded `~~~skill` guide, `--contract` discovery, self-service Bring-Your-Own-Project (BYOP) onboarding, explicit per-account credential preflight, secret-in-argv refusal, 25 MiB Drive upload validation, single-service verbs, and multi-service workflows.
- `google-rod.bsh` — Service rods for `bashy email` / `bashy drive` (`email.read`, `drive.read`, `email.send`); see [Service rods](#service-rods).
- `register.sh` — Registers `google` and its three service rods in `bashy commands` and installs the embedded skill into `bashy skill`.
- `test_cli_wrapper.py` — Hermetic offline `unittest` suite exercising onboarding, credential guards, secret refusal, single-service verbs, upload bounds, and composed workflows against a fake `gws` binary.

## Why Bring-Your-Own-Project (BYOP)?

Google classifies Gmail and Google Drive scopes as *Restricted Scopes*. Under a single shared OAuth project, Google caps unverified external apps at 100 lifetime users (requiring an annual paid CASA security audit to lift) and shares daily API quotas across all users.

Having each user or workspace tenant create their own **$0 Google Cloud project** (no billing account or credit card required) provides:
- **$0 API usage** with dedicated per-project quotas (1,000,000 Calendar queries/day, 50,000 Tasks queries/day, 12,000 Drive queries/min, 15,000 Gmail quota units/user/min).
- **Zero CASA audit requirement** and full tenant isolation.
- **Per-account credential isolation** under `${GWS_CONFIG_ROOT:-~/.config/bashy/google}/<account>/` (`0600` file permissions).

## Quick Start

### 1. Register the Command and Skill

```bash
./examples/google/register.sh
```

### 2. One-Time Self-Service Onboarding (BYOP)

```bash
# View step-by-step Google Cloud Console deep links for your account:
bashy google auth-guide account=alice@example.com project=my-bashy-gws

# Option A: Automated setup (if gcloud is installed)
bashy google auth-setup account=alice@example.com project=my-bashy-gws

# Option B: Browser-only setup (download Desktop OAuth client_secret.json from Console)
bashy google auth-import ~/Downloads/client_secret.json account=alice@example.com

# Check credential status and permissions:
bashy google auth-status account=alice@example.com

# Re-consent later (to narrow or widen permissions); refreshes credentials.json:
bashy google auth-login account=alice@example.com
```

#### Default permissions

`auth-setup`, `auth-import` and `auth-login` ask for the least that the verbs need;
pass `scopes=...` to change it.

| Service | Scope | Allows | Does not allow |
|---|---|---|---|
| Drive | `drive.file` | create folders and upload files; read and change what bashy created | seeing, changing or deleting anything else in your Drive |
| Gmail | `gmail.readonly`, `gmail.compose`, `gmail.send` | read all mail and labels, write drafts, send | modifying labels, trashing or deleting mail |
| Calendar | `calendar.events` | the calendar verbs | calendar settings and sharing |
| Tasks | `tasks` | the task verbs | |

Tasks and Calendar have no app-limited scope: `tasks` and `calendar.events` can
also delete, so bashy refuses `tasks-delete`, `calendar-delete` and
`raw ... delete|clear|trash` without `confirm=yes`.
Each API must be enabled in your project once; a 403 "API has not been used in
project" names it, e.g. `gcloud services enable drive.googleapis.com
gmail.googleapis.com calendar-json.googleapis.com tasks.googleapis.com --project P`.
Login opens Google's consent page in your browser (`GWS_NO_BROWSER=1` only prints
the link to the terminal).

`scopes=` accepts full scope URLs, short names (`drive.file`, `gmail.readonly`)
or bare service names (`scopes=gmail,drive`), which ask gws for its broader
per-service scopes, including full Drive access.

#### The `bashy` Drive folder

With `drive.file`, bashy cannot see folders you made yourself. So uploads and new
folders without `parent=` go to a `bashy` folder that bashy creates on first use
and remembers (`drive_home.json` next to the credentials). Treat it as the agents'
folder; keep your own documents in your own folders. `bashy google drive-home`
prints its link, `GWS_DRIVE_HOME=name` changes the name, and `GWS_DRIVE_HOME=`
(empty) puts them at the top of My Drive instead.

### 3. Single-Service Verbs

Set `GWS_ACCOUNT` (or pass `account=alice@example.com` per call):

```bash
export GWS_ACCOUNT="alice@example.com"

# Gmail
bashy google gmail-list q="is:unread" maxResults:=5
bashy google gmail-get 18f9a1b2c3d4e5f6
bashy google gmail-send to=bob@example.com subject="Sprint Update" body="All gates green."
bashy google gmail-draft to=bob@example.com subject="Draft Note" body="Review before sending."

# Google Calendar
bashy google calendar-list timeMin="2026-11-09T00:00:00Z" maxResults:=10
bashy google calendar-create summary="Design Sync" start="2026-11-10T10:00:00-08:00" end="2026-11-10T10:30:00-08:00" attendees:='["bob@example.com"]'
bashy google calendar-update evt_123 summary="Updated Design Sync"
bashy google calendar-freebusy timeMin="2026-11-10T00:00:00Z" timeMax="2026-11-11T00:00:00Z"

# Google Tasks
bashy google tasklists-list
bashy google tasks-list tasklist=@default showCompleted:=false
bashy google tasks-create title="Follow up on release" notes="Check benchmarks" due="2026-11-12T00:00:00Z"
bashy google tasks-complete task_987

# Google Drive
bashy google drive-list pageSize:=10
bashy google drive-search "architecture"
bashy google drive-home                     # the default "bashy" folder (created on first use)
bashy google drive-mkdir "Sprint Artifacts" # inside the bashy folder
bashy google drive-upload ./report.pdf name="Sprint-Report.pdf" parent=folder_123
```

### 4. Composed Multi-Service Workflows

```bash
# Convert a Gmail message into a linked Google Task
bashy google mail-to-task 18f9a1b2c3d4e5f6 due="2026-11-12T00:00:00Z" notes="Follow up with sender"

# Upload a file to Google Drive and email the link (with partial-mutation safety)
bashy google drive-share-mail ./report.pdf to=bob@example.com subject="Sprint Report"

# Combined daily agenda across Calendar, Tasks, and Gmail
bashy google agenda q="is:unread" maxResults:=5
```

### 5. Shaping Custom Verbs with `bashy shape`

Once registered, you can shape higher-level shortcuts over `bashy google`:

```bash
bashy shape google "add a 'standup-brief' command that runs 'agenda maxResults:=5' and projects calendar_events and tasks"
```

## Service rods

`register.sh` also registers three providers for the service verbs (Sprint
626 S5; contract: `docs/service-rod-contract.md`). They are on the `api` route,
so `bashy email` and `bashy drive` prefer the `claude:email.read` /
`claude:drive.read` connector commands when claude is installed. Pass
`--route api` or `--provider NAME` to use gws instead.

| Rod | Role | Effects | Calls |
|---|---|---|---|
| `google-email-read` | `email.read` | read,exec,net | `google gmail-list q=QUERY maxResults:=N` |
| `google-drive-read` | `drive.read` | read,exec,net | `google drive-search QUERY` (with a query) or `google drive-list` |
| `google-email-send` | `email.send` | read,exec,net,send | `google gmail-send to= subject= body=` |

```bash
bashy email --route api --json newer_than:7d                  # message ids
bashy drive --provider google-drive-read --json req.json       # {"query":"report","max":3}
bashy email --role email.send --yes --json send.json           # {"to":…,"subject":…,"body":…}
```

Read rods never change the mailbox or the Drive. The selector picks
`google-email-send` only with `--yes`. The rod also refuses on its own unless
`BASHY_SVC_YES=1`, so running `bashy google-email-send` directly sends
nothing. The rods call the registered `google` command, so the
`GWS_ACCOUNT` / `GWS_BIN` set on its record apply. Drive needs the Drive
scope in the account's OAuth grant; without it, gws answers 403
"insufficient authentication scopes". `GOWORK=off go -C examples/service-door
test ./...` (`TestGoogleRods`) covers the role mapping and the send refusals
against a fake `google`.

## Offline Verification

Run the hermetic wrapper test suite (no live Google credentials required):

```bash
python3 -m unittest examples/google/test_cli_wrapper.py
```
