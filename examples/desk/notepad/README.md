# bashy desk notepad

Notepad tool for the **bashy desk** agent-human tool suite (Sprint 537, Story #2927).

View any content as text: `txt`, `md`, `html` (converted to Markdown), `json` (pretty-printed),
and `PDF` (extracted to plain text). Humans edit text and Markdown in the loopback web page;
agents drive it via CLI commands (`bashy notepad new|open|get|put|list|share|close`) and
the identical `/api/action` JSON API.

## Features

- **Multi-format text viewing**:
  - `txt`: plain text as-is
  - `md`: Markdown as-is
  - `html`: converted to clean Markdown via `github.com/JohannesKaufmann/html-to-markdown/v2` (MIT)
  - `json`: parsed and pretty-printed with standard indentation
  - `pdf`: text extracted via pure-Go `github.com/ledongthuc/pdf` (BSD-3)
- **Desk contract compliance**:
  - Stable ID + loopback URL for every note
  - `open <id>` and `GET /api/state` return ID, URL and status only — never re-emit content
  - `get <id>` returns title, body, status and fields
  - `put <id> -f FILE|-` updates note content from file or stdin with auto-conversion
  - `list` returns note summaries without content
  - `share <id>` mints capability invite codes
  - `close <id>` freezes note against further mutations
  - Binds to `127.0.0.1` loopback only; LAN reached only via sharing
  - Rejects secret-typed fields (`kit.SecretRefused`) with 400
  - Applies security headers (`no-store`, `nosniff`, `frame-ancestors 'none'`)
- **Shared UI Kit**:
  - Built on `examples/desk/kit` (K0)
  - Copy buttons with "Copied" feedback
  - Status badges (`pending`, `needs-human`, `filled-by-agent`, `answered`, `failed`, `closed`)
  - Dark / light theme toggle
  - Responsive keyboard-friendly layout
- **Atlas & App Launcher**:
  - Atlas entry exposed via `bashy help notepad --format=mcp`
  - Web launcher tile registered via `bashy app add notepad --port 18095 --label Notepad`

## Dependencies and Licenses

Added to the `notepad` module only:
1. `github.com/JohannesKaufmann/html-to-markdown/v2` v2.5.2 — **MIT License**
2. `github.com/ledongthuc/pdf` v0.0.0-20261020125853-5e46805f718e — **BSD-3-Clause License**

## Storage

Notes persist under:
```
$BASHY_HOME/desk/notepad/items/<id>.json
```
(Defaults to `~/.local/share/bashy/desk/notepad/items/<id>.json` if `$BASHY_HOME` is unset).
Atomic writes ensure corruptions cannot occur on crash.

## CLI Usage

```bash
# Start loopback server
bashy notepad serve [-addr 127.0.0.1:18095]

# Create a new note
bashy notepad new --title "Research Notes" --body "Initial thought"
bashy notepad new -f paper.pdf --title "Paper Text"

# Re-surface note (prints id, URL, status only — never content)
bashy notepad open <id>

# Read note content
bashy notepad get <id>

# Update note content from file or stdin
bashy notepad put <id> -f summary.html
cat data.json | bashy notepad put <id> -f - --format json

# List notes
bashy notepad list [--json]

# Share note
bashy notepad share <id>

# Close note
bashy notepad close <id>
```

## Testing

```bash
cd examples/desk/notepad && GOWORK=off go test -v ./...
```
Runs the shared `desk-kit/contracttest` suite plus unit tests for content converters,
CLI duality, stdin piping, secret refusal, and server endpoints.
