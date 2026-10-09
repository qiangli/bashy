# Plan: the genie preview workflow (Sprint 379, Y7)

Goal: one documented workflow, green on macOS, Linux and Windows, against a cloud
model and a local model, both through the bashy model door.

- Doc: extend `first-party-harness.md` ("Genie preview workflow") instead of a new page.
- Script: `scripts/genie-preview-workflow.sh`: local leg first, then cloud; timeouts on
  every external step; exit 0 pass, 1 fail, 2 usage/env, 3 cloud skipped (no credential).
- The check is the file genie wrote, not its prose answer.
- `--isolate` uses a throwaway `BASHY_HOME` and door port so a shared host's door,
  bundle and sessions are untouched. Cleanup removes only what the run added.
- Verification: each OS builds bashy standalone (pins downloaded, never over an
  installed binary) and runs the script; Linux runs later on a droplet.
- Open outside this repo: `bashy llm down` is unsupported on Windows (yoke broker uses
  `Process.Signal(SIGTERM)`); the script falls back to killing the isolated door's pid.
