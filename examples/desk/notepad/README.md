# bashy desk notepad

Notepad tool for the **bashy desk** agent-human tool suite (Sprint 537, Story #2927).

View any content as text: `txt`, `md`, `html` (converted to Markdown), `json` (pretty-printed),
and `PDF` (extracted to plain text). Humans edit text and Markdown in the loopback web page;
agents drive it via CLI commands (`bashy notepad add|open|show|set|list|share|close`) and
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
  - `show <id>` returns title, body, status and fields
  - `set <id> -f FILE|-` updates note content from file or stdin with auto-conversion
  - `export <id> --to pdf|png|html|txt [-o FILE]` exports note content to PDF, PNG, HTML, or plain text
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

Added to the `notepad` module only (all pure Go, permissive):
1. `github.com/JohannesKaufmann/html-to-markdown/v2` v2.5.2 — **MIT License** (HTML -> Markdown input conversion)
2. `github.com/ledongthuc/pdf` v0.0.0-20261020125853-5e46805f718e — **BSD-3-Clause License** (PDF -> text input extraction)
3. Markdown rendering and HTML export use the shared kit renderer (`desk-kit/render`, goldmark **MIT License**) — the live preview, viewer and export share one implementation, so blackfriday is gone
4. `github.com/go-pdf/fpdf` v0.9.0 — **MIT License** (text/Markdown -> PDF export writer)
5. `golang.org/x/image` v0.47.0 — **BSD-3-Clause License** (text/Markdown -> PNG font drawing & export writer)

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

# Create a note
bashy notepad add --title "Research Notes" --body "Initial thought"
bashy notepad add -f paper.pdf --title "Paper Text"

# Re-surface note (prints id, URL, status only — never content)
bashy notepad open <id>

# Read note content
bashy notepad show <id>

# Update note content from file or stdin
bashy notepad set <id> -f summary.html
cat data.json | bashy notepad set <id> -f - --format json

# Export note content to PDF, PNG, HTML, or TXT
bashy notepad export <id> --to pdf [-o output.pdf]
bashy notepad export <id> --to png
bashy notepad export <id> --to html
bashy notepad export <id> --to txt

# List notes
bashy notepad list [--json]

# Share note
bashy notepad share <id>

# Close note
bashy notepad close <id>
```

## As a shaped app

`shaped.json` declares notepad's server, so it can be registered as a shaped
app and get per-user source history, preview, restore and undo:

```bash
bashy app add notepad --type shaped --source "$PWD"   # from this directory
bashy src history notepad --user device:alice
```

The console builds the server from each user's active revision (`go build
-trimpath`, `GOWORK=off`, `CGO_ENABLED=0`), starts it on a loopback port it
picks with that user's app data as `-data`, and proxies `/notepad/` to it.
`app add` vendors the out-of-tree `desk-kit => ../kit` replace into the
managed checkout so every revision builds from its own tree. The page calls
`api/action` relative to itself so it works at `/` and under a mount, and
item URLs follow the mount (`X-Forwarded-Prefix`) instead of naming the
server's internal loopback address.

## Testing

```bash
cd examples/desk/notepad && GOWORK=off go test -v ./...
```
Runs the shared `desk-kit/contracttest` suite plus unit tests for content converters,
CLI duality, stdin piping, secret refusal, and server endpoints.
