#!/usr/bin/env bash
# bashy sprint recipes; save as a .sh file and run with one task name.
# Set the RECIPE_* variables named by that task before running.
case "${1:-}" in
  parse-output) # parse output
    bashy adapter list --json
    ;;
  keep-only-some-fields) # keep only some fields
    bashy cut --delimiter=, --fields=1,2 "${RECIPE_FILE:?set RECIPE_FILE}"
    ;;
  nul-safe-lists) # NUL-safe lists
    bashy find "${RECIPE_PATH:-.}" -print0
    ;;
  see-what-would-change) # see what would change
    bashy rm -r --dry-run --no-input "${RECIPE_PATH:?set RECIPE_PATH}"
    ;;
  avoid-hanging-on-a-prompt) # avoid hanging on a prompt
    bashy rm -i --dry-run --no-input "${RECIPE_PATH:?set RECIPE_PATH}"
    ;;
  understand-a-command-before-running-it) # understand a command before running it
    bashy adapt --explain -- rsync -a "${RECIPE_SOURCE:?set RECIPE_SOURCE}" "${RECIPE_DESTINATION:?set RECIPE_DESTINATION}"
    ;;
  read-a-pdf-docx-html-csv) # read a pdf/docx/html/csv
    bashy lens -t md "${RECIPE_FILE:?set RECIPE_FILE}"
    ;;
  convert-json-to-yaml-and-back) # convert json to yaml and back
    bashy lens -f json -t yaml < "${RECIPE_FILE:?set RECIPE_FILE}"
    ;;
  list-an-archive-or-container) # list an archive or container
    bashy lens -t listing "${RECIPE_FILE:?set RECIPE_FILE}"
    ;;
  show-a-command-for-powershell-cmd-mcp-or-a-skill) # show a command for PowerShell, cmd, MCP or a skill
    bashy lens -f bash -t pwsh --switch=pascal < "${RECIPE_FILE:?set RECIPE_FILE}"
    ;;
  read-file-at-a-commit-without-checkout) # read file at a commit without checkout
    bashy lens -t md "git:${RECIPE_REV:?set RECIPE_REV}:${RECIPE_PATH:?set RECIPE_PATH}"
    ;;
  use-any-common-option-on-a-foreign-command-git-tar-apt-docker) # use any common option on a foreign command (git, tar, apt, docker…)
    bashy adapt --explain -- git status
    ;;
  *) printf 'Available tasks:\n' >&2
     printf '  parse-output\n' >&2
     printf '  keep-only-some-fields\n' >&2
     printf '  nul-safe-lists\n' >&2
     printf '  see-what-would-change\n' >&2
     printf '  avoid-hanging-on-a-prompt\n' >&2
     printf '  understand-a-command-before-running-it\n' >&2
     printf '  read-a-pdf-docx-html-csv\n' >&2
     printf '  convert-json-to-yaml-and-back\n' >&2
     printf '  list-an-archive-or-container\n' >&2
     printf '  show-a-command-for-powershell-cmd-mcp-or-a-skill\n' >&2
     printf '  read-file-at-a-commit-without-checkout\n' >&2
     printf '  use-any-common-option-on-a-foreign-command-git-tar-apt-docker\n' >&2
     exit 2
     ;;
esac

# Usage
# sprint is the conductor's PLAN/HANDOFF layer — the cross-repo kanban
# See `bashy help common-options` for vocabulary.
