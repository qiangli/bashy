# Google Workspace Custom Command (`examples/google`)

This example exposes Google Workspace services (**Gmail**, **Google Calendar**, **Google Tasks**, and **Google Drive**) inside `bashy` through a self-contained Bash# custom command (`google.bsh`) and embedded skill wrapping the `gws` (`googleworkspace/cli`) binary.

## Files

- `google.bsh` — Executable Bash# script (`#!/usr/bin/env -S bashy --bashsharp`) with embedded `~~~skill` guide, `--contract` discovery, self-service Bring-Your-Own-Project (BYOP) onboarding, explicit per-account credential preflight, secret-in-argv refusal, 25 MiB Drive upload validation, single-service verbs, and multi-service workflows.
- `register.sh` — Registers `google` in `bashy commands` and installs the embedded skill into `bashy skill`.
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
```

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
bashy google drive-mkdir "Sprint Artifacts"
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

## Offline Verification

Run the hermetic wrapper test suite (no live Google credentials required):

```bash
python3 -m unittest examples/google/test_cli_wrapper.py
```
