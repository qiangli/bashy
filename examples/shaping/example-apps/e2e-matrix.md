# Member-principal consumer-shaping E2E across the example apps (Sprint 682 C4)

`e2e-agent.sh` here is the app-matrix sibling of
`../hello-world/e2e-agent.sh`: the same stdin launcher contract and the same
disposable-installation rules (read `../hello-world/e2e-matrix.md` first;
everything there about isolated `HOME`/`BASHY_HOME`/`BASHY_FLEET_DIR`,
pinned executables, paired browser profiles and sanitized evidence applies
unchanged). The difference is the variable: hello-world varies the
interface (CLI/MCP/REST) and agent over one app; this script fixes the CLI
interface and varies the **app**, one real-agent invocation per app.

Each row drives the full member lifecycle as principal A
(`E2E_PRINCIPAL_A`, default `m_alice`): status → seed through the app's
shipped CLI → wish (the app's first shipped `genie.examples` entry,
verbatim) → show/diff → verify → preview → approve → principal B
(`E2E_PRINCIPAL_B`, default `m_bob`) still baseline → undo → baseline
restored with seeded data intact.

## Rows

| App | Wish (first shipped `genie.examples`) | Assertion | Gate |
|---|---|---|---|
| form | Keep the Save and Submit bar visible while I scroll through a long questionnaire. | Bar sticky/fixed + Save works on a long form | — |
| notepad | Show live word count and estimated reading time in the editor header. | Header count 120 → 150 after appending 30 words | — |
| session | Show who is overdue first | Overdue rows sort first for A only | — |
| viewer | Keep the page navigation visible while I scroll through a long report. | Nav sticky/fixed at page end; paging works | — |
| chart | Show the minimum, maximum and average above the line chart. | Series 1,2,3,4 → min 1, max 4, avg 2.5 shown | — |
| board | Show my cards first | A's assigned cards first in the fixture column | operator pre-install |
| whiteboard | Make the canvas background darker | Computed background luminance below baseline; scene intact | operator pre-install |
| table | Show my records in a compact grid | Row height/density more compact than baseline | operator pre-install, Linux only |

The wishes are duplicated in the script's case table;
`bash e2e-agent.sh --dry-run` fails if any of them drifts from the app's
`shaped.json`, so the table cannot silently rot.

## Preparing a run (operator, on the gate host)

1. Create the disposable installation exactly as in the hello-world
   matrix: fresh `HOME`, `BASHY_HOME`, `BASHY_FLEET_DIR`, pinned Bashy as
   `bashy` on a private PATH, writable disposable tracking store, and two
   paired synthetic principals in fresh browser profiles.
2. Register each app under test in that installation with its shipped
   `register.sh` (all go through the public
   `bashy app add NAME --type shaped --source DIR` path; board, whiteboard
   and table register a custom command instead — run their `<app> install`
   yourself if you want those rows live; the script never installs and
   never touches the network, it SKIPs them loudly instead).
3. For board/whiteboard/table, prepare the fixture each row names (seeded
   board, saved scene, ≥10-record table) and record it in the context file.
4. Write the private `E2E_CONTEXT` file as in hello-world (resource
   references only — no cookie, token or credential values), including the
   fixture identities from step 3 and the paired browser profiles.
5. Set the shaping-agent provenance the wishes will carry, then run:

```sh
export BASHY_BIN=/path/to/pinned/bashy
export BASHY_HOME=/path/to/disposable/state
export BASHY_FLEET_DIR=/path/to/disposable/catalog
export E2E_CONTEXT=/path/to/private/run-context.md
export E2E_AGENT_ID=tool:model            # e.g. the TOOL:MODEL you configured
export E2E_EVIDENCE_DIR=/path/to/evidence # optional; a temp dir is used otherwise
bash e2e-agent.sh --dry-run               # authoring check: renders, runs nothing
bash e2e-agent.sh --app form codex exec - # one row
bash e2e-agent.sh codex exec -            # all rows, sequentially
```

Agent CLIs that do not accept stdin need the same operator wrapper the
hello-world matrix describes. Do not substitute an echo provider: a row
only counts when a real agent produced the candidate through the shaping
service.

## Reading the result

Per app the script prints exactly one of:

- `E2E_<APP>_PASS` — the agent reported every step and assertion passed.
  As in hello-world this is a claim to review, not acceptance: the
  conductor checks the retained per-app evidence log.
- `E2E_<APP>_BLOCKED: <reason>` — a prerequisite, the agent run, or an
  intermediate gate failed before a trustworthy verdict.
- `E2E_<APP>_SKIP: <reason>` — the row is not runnable on this host
  (install-wrapped app not pre-installed, or table off Linux). A skip is
  recorded with its reason, never folded into a pass.

Exit codes: `0` every selected row PASS; `2` usage or blocked preflight;
`3` a row FAILED/BLOCKED; `4` rows skipped and none failed. Browser
assertions must come from a real browser paired as each principal;
narration in the agent log is not browser evidence.
