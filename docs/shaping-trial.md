# Shaping the bashy app web UI: opt-in trial guide

Software Shaping lets each user change an app in plain words. Genie writes the
change, the trusted verifier checks it, and only that user sees the result
until they share it. This guide is the opt-in trial procedure for the
`bashy app` web UI (the launcher, app name `console`) and for the desk
`notepad` and `form` apps.

Status: trial readiness. Nothing here is turned on for anyone who does not
follow these steps.

## What can and cannot be shaped

Shapable (per user): the launcher pages `index`, `desktop`, `board`, `inbox`
and `mb` (HTML, CSS, JS), and the desk `notepad` and `form` apps.

Never shapable (the base copy is always served): launcher sign-in, pairing,
device grants and the gate; the `shape` review page; the `term` page (shell
access); the `accounts` page (credentials); every Go API handler. A shaped
version that tries to replace or drop one of these is refused.

## Prerequisites

- A bashy build that includes Sprint 526 (`bashy --version`).
- A shaping agent the host can reach, for example a model door shared from
  another of your machines with `bashy llm share <this-host>`. Check it with
  `bashy genie -m <model>@<door-host> "Reply PONG"`.

## Try it

1. Start the console: `bashy app serve --pair --shape-agent <tool:model>`.
2. Pair your browser or device with the launcher in scope:
   `bashy app pair --principal <you> --allow console` and open the printed URL.
3. Open `/shape/console/` (the sparkle button in the launcher header) and
   describe the change you want, in plain words.
4. Review the candidate: read the diff and run verify. Approve only what you
   want; reject leaves nothing behind.
5. Your launcher now shows your version; other users still see theirs.

Desk apps: register them with `examples/desk/notepad/register.sh` and
`examples/desk/form/register.sh` (both use the public `bashy app add --type
shaped` path), then shape them the same way at `/shape/notepad/` and
`/shape/form/`.

## Versions, releases, sharing and cleanup

- History: every approved change is a version. Restore an older one or undo
  from the shaping page, or with `bashy src history` / `bashy src restore`.
- Releases: `bashy src release <app> <rev>` tags a semver release: a repair
  is a patch bump, an added feature a minor bump, and a removal or rename a
  major bump, only through an explicit contract step. Untagged versions show
  pseudo-versions.
- Sharing: `bashy src offer <app> <rev> --to <user>` publishes a released
  version. The other user previews it with `bashy src adopt <app> <offer>
  --preview`, then adopts it; it is verified again for them before it is
  activated.
- Cleanup: `bashy app tidy <app> --user <you>` previews which old versions
  would be compacted; `--apply` does it. Named, adopted, offered and released
  versions are always kept.

## Safe changes

Genie is told to add rather than remove: new properties and elements keep a
fallback for older data. The verifier refuses a version that removes or
renames a declared route, unit, flag, argument or stored property, and says
why. Removing something needs a separate step you approve explicitly.

## Troubleshooting

- `--allow console` refused: the build predates the fix; update bashy.
- A wish fails with `delegate: ... exited 124`: the agent was silent past the
  idle limit. Update bashy (15-minute idle default) or check the agent.
- A wish fails with connection errors: check the model door
  (`bashy genie -m <model>@<door-host> "Reply PONG"`); re-run
  `bashy llm share` from the door host after moving machines between
  satellites.
- Approve is refused with a verifier reason: read it. It names the check
  that failed or the element that would be removed.

## Rollback and leaving the trial

- Undo or restore any earlier version from the shaping page.
- To go back to the shipped launcher entirely, restore the base version
  (`base`). The launcher always falls back to the built-in pages when you
  have no active version.
- Restoring `base` affects only you; other users keep their own versions.
  Do not delete the app's source checkout under `~/.bashy/src/`: it is
  shared by every user of the app.

## Feedback

Feedback is voluntary. Tell us what you shaped, what worked and what did
not, through your usual bashy channel. The trial collects no usage data and
sends no telemetry.

## Announcement copy (draft, not sent)

> **Shape the bashy app UI your way.** Describe a change in plain words. Genie
> writes it, bashy checks it, and only you see it until you share it. Every
> change is a version you can undo. Shipped pages, sign-in and credentials
> stay protected. Opt in with `docs/shaping-trial.md`.
