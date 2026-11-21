#!/usr/bin/env bash
# Sprint 682 C4: member-principal consumer-shaping E2E across the shaped
# example apps, modeled on ../hello-world/e2e-agent.sh (same stdin launcher
# contract, same disposable-installation preconditions — see e2e-matrix.md).
#
# CLI interface only: hello-world already covers CLI/MCP/REST transport
# parity; this script varies the app. One real-agent invocation per app, so
# each app yields its own evidence and marker. The script never installs
# anything and never touches the network: install-wrapped apps (board,
# whiteboard, table) must be installed and registered by the operator
# beforehand, or their row is an explicit SKIP.
#
# Usage:
#   bash e2e-agent.sh [--app APP|all] [--dry-run] [AGENT [ARGS...]]
#   bash e2e-agent.sh --app form codex exec -
#   bash e2e-agent.sh --dry-run            # render + validate, runs nothing
#
# Exit codes: 0 every selected row PASS; 2 usage / blocked preflight;
# 3 a row FAILED or BLOCKED; 4 rows were SKIPPED and none failed.
# Agent exit zero is execution, not acceptance: a row counts only when the
# agent's output carries its exact E2E_<APP>_PASS marker, and the conductor
# still reviews the retained evidence (as in hello-world).
set -uo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../../.." && pwd)
runner=${BASHY_BIN:-bashy}
principal_a=${E2E_PRINCIPAL_A:-m_alice}
principal_b=${E2E_PRINCIPAL_B:-m_bob}
agent_id=${E2E_AGENT_ID:-}

ALL_APPS="form notepad session viewer chart board whiteboard table"

usage() {
  echo 'Usage: bash e2e-agent.sh [--app APP|all] [--dry-run] [AGENT [ARGS...]]' >&2
  echo "Apps: $ALL_APPS" >&2
  echo 'The agent command must accept the task on stdin; prepare a disposable' >&2
  echo 'installation first (see e2e-matrix.md). --dry-run renders the prompts' >&2
  echo 'and checks every wish against the shipped shaped.json; it runs nothing.' >&2
}

# Per-app case table. WISH is the app's first shipped genie.examples entry,
# verbatim; --dry-run fails if it drifts from shaped.json. GATE: none |
# install (operator must pre-install) | install-linux (also Linux-only).
app_case() {
  case "$1" in
    form)
      SRC=examples/desk/form; GATE=none
      WISH='Keep the Save and Submit bar visible while I scroll through a long questionnaire.'
      SEED='With the shipped form CLI (discover it via its --help; it follows the console runtime address, an explicit FORM_ADDR-style handoff wins), create a questionnaire long enough that the Save/Submit bar starts off-screen, and open it as principal A.'
      ASSERT='In the browser paired as A: after scrolling to the last field the Save/Submit bar is still visible (computed position sticky or fixed) and Save succeeds. For B the bar keeps its baseline position behavior.'
      ;;
    notepad)
      SRC=examples/desk/notepad; GATE=none
      WISH='Show live word count and estimated reading time in the editor header.'
      SEED='With the shipped notepad CLI (console runtime discovery; explicit handoff wins), create a note whose body is exactly 120 words, and open it as principal A.'
      ASSERT='The editor header shows a word count of 120 and a reading-time estimate for A; appending 30 more words updates the count to 150 without a reload. B sees no header stats.'
      ;;
    session)
      SRC=examples/session; GATE=none
      WISH='Show who is overdue first'
      SEED='With the shipped session CLI in console mode (SESSION_ADDR/SESSION_DATA handoff wins, else `bashy app show session --runtime --json`), register a session whose rows include at least one overdue and one current entry, and open its display page as A.'
      ASSERT='Overdue rows render before non-overdue rows for A. B sees the baseline order. The seeded rows are unchanged after undo.'
      ;;
    viewer)
      SRC=examples/desk/viewer; GATE=none
      WISH='Keep the page navigation visible while I scroll through a long report.'
      SEED='With the shipped viewer CLI (bashy viewer add/append; runtime discovery via `bashy app show viewer --runtime --json`, explicit handoff wins), create a book and append a Markdown page long enough to scroll several screens, then open it as A.'
      ASSERT='After scrolling to the end of the page as A, the page navigation is still visible (computed position sticky or fixed) and paging still works. B keeps the baseline navigation behavior.'
      ;;
    chart)
      SRC=examples/desk/chart; GATE=none
      WISH='Show the minimum, maximum and average above the line chart.'
      SEED='With the shipped chart CLI (bashy chart add/open; runtime discovery via `bashy app show chart --runtime --json`, explicit handoff wins), add a line chart with the series 1, 2, 3, 4 and open it as A.'
      ASSERT='Above the chart A sees minimum 1, maximum 4 and average 2.5 (any reasonable formatting of those three values). B sees the chart without the stats line.'
      ;;
    board)
      SRC=examples/board; GATE=install
      WISH='Show my cards first'
      SEED='Use the operator-prepared board fixture named in the run context: one column holding cards assigned to A and to someone else, with an A card not initially first. Open the board as A.'
      ASSERT='In that column the cards assigned to A render before the others for A. For B the order is the baseline order. Card contents are unchanged after undo.'
      ;;
    whiteboard)
      SRC=examples/whiteboard; GATE=install
      WISH='Make the canvas background darker'
      SEED='Use the operator-prepared whiteboard fixture named in the run context (a saved scene with at least one element). Record the baseline computed canvas background color, then open the board as A.'
      ASSERT='The computed canvas background color for A is darker than the recorded baseline (lower luminance), and the seeded element is still present. B keeps the baseline background.'
      ;;
    table)
      SRC=examples/table; GATE=install-linux
      WISH='Show my records in a compact grid'
      SEED='Use the operator-prepared table fixture named in the run context (a table with at least ten records). Record the baseline computed row height, then open it as A.'
      ASSERT='The computed row height (or the grid density class, if rows are virtualized) is more compact than baseline for A, with the same records visible. B keeps the baseline density.'
      ;;
    *) return 1 ;;
  esac
}

marker() { printf 'E2E_%s_PASS' "$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]')"; }

skip_reason() { # app -> prints reason and returns 0 when the row must skip
  app_case "$1" >/dev/null || return 1
  if [ "$GATE" = install-linux ] && [ "$(uname -s)" != Linux ]; then
    echo "table is Linux-only (this host is $(uname -s)); see plan appendix on install-wrapped apps"
    return 0
  fi
  return 1
}

render_prompt() { # app
  local app=$1
  app_case "$app"
  local pass; pass=$(marker "$app")
  cat <<EOF
# Consumer-shaping member E2E: $app (Sprint 682 C4, CLI interface)

Selected interface: CLI. Use the candidate executable in BASHY_BIN, also
available as \`bashy\` on PATH. Invoke \`bashy shape\` directly for every
shaping operation; discover its help. Do not use --remote, MCP or raw REST
as substitutes. You may prepare the requested change in a separate
disposable candidate workspace and import it with
\`bashy shape wish APP REQUEST --from WORKSPACE\`. Never edit the deployed
app, the managed active store, or a shared source checkout directly. Do not
expose credentials in prompts or logs, and never print a full URL, host,
path, cookie or token in evidence.

Principals: A = $principal_a (the member who shapes), B = $principal_b
(must stay on the baseline version throughout). Shaping-agent provenance:
--agent $agent_id

App under test: $app (registered name), source $SRC.
The member wish, verbatim from the app's shipped genie.examples:
  $WISH

Seed: $SEED
Acceptance assertion: $ASSERT

Steps:
1. \`bashy shape status $app --user $principal_a --json\`: record the baseline
   active version and confirm no candidate is pending. In a real browser
   paired as A, confirm the baseline surface renders and the Genie controls
   are injected through the console app mount. If an operation or the
   browser is unavailable, stop and report BLOCKED with the exact missing
   dependency.
2. Seed as above, through the app's shipped agent CLI only. The CLI must
   reach the console-launched instance (runtime record or explicit
   handoff); if it falls back to a fixed port or its own server, stop:
   that is a FAIL of the app, not a setup detail.
3. \`bashy shape wish $app '<the wish above>' --user $principal_a --agent $agent_id --json\`.
   Inspect the candidate with show and diff. The diff must stay inside the
   app's own source/UI layer: no other app, no registrations, no shared
   kit, no data files.
4. \`bashy shape verify $app CANDIDATE --user $principal_a\`. On any failure stop;
   never approve a failed candidate.
5. \`bashy shape preview $app CANDIDATE --user $principal_a\`. In the browser paired
   as A, check the acceptance assertion in preview; the active app must
   still show baseline. Narration is not browser evidence: retain the
   assertion output.
6. \`bashy shape approve $app CANDIDATE --user $principal_a\`. Re-check the
   acceptance assertion on the active app as A, then check as B (status
   and surface): B must still be on the baseline version and behavior.
7. \`bashy shape undo $app --user $principal_a\`. Assert the baseline version and
   behavior are restored for A and the seeded data is still present.

Report each step PASS, FAIL or BLOCKED with the actual commands and
sanitized structured output. End your report with exactly one final line:
$pass when every step and assertion passed, otherwise
E2E_$(printf '%s' "$app" | tr '[:lower:]' '[:upper:]')_BLOCKED: <one-line reason>.
Exit zero from your process is execution, not acceptance; the conductor
reviews the evidence.
EOF
  if [ -n "${E2E_CONTEXT:-}" ] && [ -f "${E2E_CONTEXT:-}" ]; then
    printf '\nOperator-prepared disposable run context:\n'
    cat "$E2E_CONTEXT"
  fi
}

selected=all
dry_run=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --app) [ "$#" -ge 2 ] || { usage; exit 2; }; selected=$2; shift 2 ;;
    --dry-run) dry_run=1; shift ;;
    --) shift; break ;;
    -*) usage; exit 2 ;;
    *) break ;;
  esac
done

case " $ALL_APPS all " in
  *" $selected "*) ;;
  *) echo "BLOCKED: unknown app '$selected' (want one of: $ALL_APPS, all)" >&2; exit 2 ;;
esac
apps=$ALL_APPS
[ "$selected" != all ] && apps=$selected

if [ "$dry_run" -eq 1 ]; then
  [ "$#" -eq 0 ] || { echo 'BLOCKED: --dry-run takes no agent command.' >&2; exit 2; }
  agent_id=${agent_id:-tool:model}
  rc=0
  for app in $apps; do
    app_case "$app"
    if reason=$(skip_reason "$app"); then
      printf 'E2E_%s_SKIP: %s\n' "$(printf '%s' "$app" | tr '[:lower:]' '[:upper:]')" "$reason"
    fi
    manifest="$root/$SRC/shaped.json"
    if [ ! -f "$manifest" ]; then
      echo "DRY-RUN FAIL $app: manifest not found at $SRC/shaped.json" >&2; rc=3
    elif ! grep -qF -- "$WISH" "$manifest"; then
      echo "DRY-RUN FAIL $app: case wish is not in $SRC/shaped.json genie.examples" >&2; rc=3
    else
      echo "DRY-RUN OK $app: wish matches $SRC/shaped.json; gate=$GATE; marker=$(marker "$app")"
    fi
    printf -- '--- prompt: %s ---\n' "$app"
    render_prompt "$app"
    printf -- '--- end prompt: %s ---\n' "$app"
  done
  echo 'Dry run only: nothing was registered, wished, verified or opened.' >&2
  exit "$rc"
fi

# Live preflight, mirroring hello-world: disposable installation only.
if [ -z "${BASHY_HOME:-}" ] || [ ! -d "$BASHY_HOME" ]; then
  echo 'BLOCKED: set BASHY_HOME to the prepared disposable installation directory.' >&2; exit 2
fi
if [ -z "${BASHY_FLEET_DIR:-}" ] || [ ! -d "$BASHY_FLEET_DIR" ]; then
  echo 'BLOCKED: set BASHY_FLEET_DIR to the prepared disposable catalog directory.' >&2; exit 2
fi
if [ -z "${E2E_CONTEXT:-}" ] || [ ! -f "$E2E_CONTEXT" ]; then
  echo 'BLOCKED: E2E_CONTEXT must name the operator-prepared run context (see e2e-matrix.md).' >&2; exit 2
fi
if [ -z "$agent_id" ]; then
  echo 'BLOCKED: set E2E_AGENT_ID to the shaping agent provenance (tool:model).' >&2; exit 2
fi
if ! "$runner" shape --help >/dev/null 2>&1; then
  echo 'BLOCKED: this Bashy build lacks the shaping CLI.' >&2; exit 2
fi
if [ "$#" -eq 0 ]; then
  usage
  echo 'Example: bash e2e-agent.sh --app form codex exec -' >&2
  exit 2
fi

evidence_dir=${E2E_EVIDENCE_DIR:-$(mktemp -d)}
mkdir -p "$evidence_dir"
overall=0
for app in $apps; do
  upper=$(printf '%s' "$app" | tr '[:lower:]' '[:upper:]')
  if reason=$(skip_reason "$app"); then
    printf 'E2E_%s_SKIP: %s\n' "$upper" "$reason"
    [ "$overall" -eq 0 ] && overall=4
    continue
  fi
  app_case "$app"
  # Registration/status probe: install-gated apps skip loudly when the
  # operator has not pre-installed them; the rest are BLOCKED, not skipped.
  if ! "$runner" shape status "$app" --user "$principal_a" --json >/dev/null 2>&1; then
    if [ "$GATE" != none ]; then
      printf 'E2E_%s_SKIP: %s is not installed/registered as a shaped app on this host (operator pre-install required; this script never installs)\n' "$upper" "$app"
      [ "$overall" -eq 0 ] && overall=4
    else
      printf 'E2E_%s_BLOCKED: shape status failed for %s as %s; register it in the disposable installation first (see e2e-matrix.md)\n' "$upper" "$app" "$principal_a"
      overall=3
    fi
    continue
  fi
  out="$evidence_dir/$app.agent.log"
  render_prompt "$app" | "$@" >"$out" 2>&1
  agent_rc=$?
  if [ "$agent_rc" -ne 0 ]; then
    printf 'E2E_%s_BLOCKED: agent command exited %s; evidence: %s\n' "$upper" "$agent_rc" "$out"
    overall=3
  elif grep -qF -- "$(marker "$app")" "$out"; then
    printf '%s (evidence: %s; conductor review still required)\n' "$(marker "$app")" "$out"
  else
    printf 'E2E_%s_BLOCKED: agent finished without the %s marker; evidence: %s\n' "$upper" "$(marker "$app")" "$out"
    overall=3
  fi
done
echo "Evidence directory: $evidence_dir" >&2
echo 'Agent runs finished; E2E acceptance requires review of every assertion and evidence artifact.' >&2
exit "$overall"
