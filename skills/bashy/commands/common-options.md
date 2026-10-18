---
name: common-options
description: The closed long-only common-options vocabulary (--json, --fields, --null, --dry-run, --no-input, --explain, --depth, --since) that works the same on every bashy-owned command and via bashy adapt for foreign commands.
---

# common-options

One consistent long-only vocabulary on every bashy-owned command, extended to every file (`lens`) and every command (`adapt`), with classic POSIX byte-identical under certification.

| option | meaning | when no native mapping (native exit 2; adapt exits 125) |
|---|---|---|
| `--json` | machine output: one JSON value, or NDJSON for streams | use declared lens parser row `cmd:<name>`, else exit 2 |
| `--fields=a,b` | project JSON records | generic, after `--json` |
| `--null` | NUL-separated records (`-0` only where standard) | map to native `-z`/`--zero`/`-print0`, else exit 2 |
| `--dry-run` | report what would change; change nothing (preview_level) | shared effect seam, else exit 2 |
| `--no-input` | never prompt; fail where a prompt would happen | engine marks stdin non-interactive |
| `--explain` | describe each flag/operand and declared effects; never run | generic from derived schema |
| `--depth=N` | bound a tree walk | providers and tree-shaped commands |
| `--since=CURSOR` | filter after cursor envelope (bashy-cursor-v1) | versioned providers, else exit 2 |
| `--help=skill|classic|mcp` | help in reader format | generic |
| `--verbose`/`--quiet`/`--debug` | diagnostics on stderr only | naming rule only |

Two spellings per option: the flag on any bashy-owned command, or `bashy adapt --flag -- CMD` for anything. `adapt` pairs with `adapter` rows (fleet kind `adapter`, curated defaults for git, tar, unzip, docker, kubectl, rsync, make, apt). `lens` is the representation filter (iconv for formats).

See `docs/sprint-446-master-execution-plan.md`, `bashy help adapt`, `lens -t listing`, and the trigger table in `internal/agentos/adoption.go`.
