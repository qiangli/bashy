---
name: bashy-sprint-recipes
description: S9 trigger recipes for bashy sprint
type: dag
default: parse-output
---

# bashy sprint recipes

Source: `bashy help --format=dag sprint`

## Tasks

### parse-output
parse output

```bash
bashy adapter list --json
```

### keep-only-some-fields
keep only some fields

```bash
bashy cut --delimiter=, --fields=1,2 "${RECIPE_FILE:?set RECIPE_FILE}"
```

### nul-safe-lists
NUL-safe lists

```bash
bashy find "${RECIPE_PATH:-.}" -print0
```

### see-what-would-change
see what would change

```bash
bashy rm -r --dry-run --no-input "${RECIPE_PATH:?set RECIPE_PATH}"
```

### avoid-hanging-on-a-prompt
avoid hanging on a prompt

```bash
bashy rm -i --dry-run --no-input "${RECIPE_PATH:?set RECIPE_PATH}"
```

### understand-a-command-before-running-it
understand a command before running it

```bash
bashy adapt --explain -- rsync -a "${RECIPE_SOURCE:?set RECIPE_SOURCE}" "${RECIPE_DESTINATION:?set RECIPE_DESTINATION}"
```

### read-a-pdf-docx-html-csv
read a pdf/docx/html/csv

```bash
bashy lens -t md "${RECIPE_FILE:?set RECIPE_FILE}"
```

### convert-json-to-yaml-and-back
convert json to yaml and back

```bash
bashy lens -f json -t yaml < "${RECIPE_FILE:?set RECIPE_FILE}"
```

### list-an-archive-or-container
list an archive or container

```bash
bashy lens -t listing "${RECIPE_FILE:?set RECIPE_FILE}"
```

### show-a-command-for-powershell-cmd-mcp-or-a-skill
show a command for PowerShell, cmd, MCP or a skill

```bash
bashy lens -f bash -t pwsh --switch=pascal < "${RECIPE_FILE:?set RECIPE_FILE}"
```

### read-file-at-a-commit-without-checkout
read file at a commit without checkout

```bash
bashy lens -t md "git:${RECIPE_REV:?set RECIPE_REV}:${RECIPE_PATH:?set RECIPE_PATH}"
```

### use-any-common-option-on-a-foreign-command-git-tar-apt-docker
use any common option on a foreign command (git, tar, apt, docker…)

```bash
bashy adapt --explain -- git status
```
