# Local Knowledge UI (`kb-ui`)

A local-knowledge monitor, graph explorer, and secure document preview interface for Bashy.

Ported from `kg-ui` (`/Users/qiangli/projects/poc/dhnt/kg/cmd/kg-ui`) under explicit operator authorization. Operates completely standalone with zero dependencies on external kg stores or daemons, connecting directly to public Bashy search and local knowledge APIs via a thin adapter.

## Key Features

- **Explorer & Graph UI**: Search indexed documents, view metadata, and inspect relationships across local knowledge roots.
- **Real-Time Monitor**: Live status events over a reconnecting SSE stream (`/events`), showing indexing progress, rates, and storage usage.
- **Secure File Preview**: Sandboxed viewer for eligible text and markdown files under explicitly configured roots.
- **Root Management**: Manage multiple knowledge roots with home-path expansion (`~`), visible includes/excludes, progress tracking, and pause/resume controls.
- **Offline Assets**: All frontend scripts and styles are embedded (`three.js`, `3d-force-graph`, `vis-network`), requiring no external network access.
- **Explicit Telemetry**: Clearly presents actual backend state without fabricating mock data; unsupported legacy enrichment or queue functions are explicitly surfaced.

## Security Architecture

1. **Loopback-Only Bind**: By default listens strictly on `127.0.0.1:6674`, preventing exposure to local networks.
2. **File Preview Sandboxing**:
   - Only files residing within explicitly registered roots may be previewed or downloaded.
   - Symlinks are evaluated and verified to remain strictly within the registered roots.
   - Traversal attempts (`../`), sensitive files (`.git`, `.ssh`, `.env`, credentials, private keys), and system paths are rejected with `403 Forbidden`.
   - Bounded response sizes (capped at 2 MB) and MIME detection (binary files return 415 without loading into the text viewer).
3. **Mutation Defense (CSRF & Cross-Origin)**:
   - State-changing requests (`POST`, `DELETE`, `PUT`) validate `Sec-Fetch-Site` (blocking `cross-site`).
   - `Origin` and `Referer` headers are validated against the server's listener host.
   - Requires custom anti-CSRF headers (`X-KB-UI: 1` or `application/json`), blocking form-based or unauthorized browser scripts.
4. **Base Escaping**:
   - `X-Forwarded-Prefix` values are sanitized and HTML-attribute escaped before insertion into `<base href="...">`, preventing attribute injection.

## Running with `bashy dag`

Driven via `examples/local-knowledge/dag.md`:

```sh
# Build the binary
bashy dag -f examples/local-knowledge/dag.md build

# Run unit and HTTP boundary tests
bashy dag -f examples/local-knowledge/dag.md test

# Smoke-test the health and root endpoints
bashy dag -f examples/local-knowledge/dag.md smoke

# Run the UI server
bashy dag -f examples/local-knowledge/dag.md run
```

## Running directly with Go

```sh
# Run server with current directory and notes as roots
go run ./examples/local-knowledge/kb-ui --addr 127.0.0.1:6674 --roots ~/notes,.
```

Open `http://127.0.0.1:6674/` in your browser.

## HTTP Endpoints

| Route | Method | Description |
|---|---|---|
| `/` | `GET` | Embedded single-page application |
| `/events` | `GET` | Server-Sent Events (SSE) status stream |
| `/healthz` | `GET` | Health check endpoint (`{"ok":true}`) |
| `/api/search` | `GET` | Search indexed documents (`?q=...&mode=...`) |
| `/api/file` | `GET` | Secure document preview (`?uri=...&download=1`) |
| `/api/markdown` | `GET` | Document preview for markdown text |
| `/api/nodes/*` | `GET` | Detailed node record for graph exploration |
| `/api/roots` | `GET` | List registered indexing roots |
| `/api/roots` | `POST` | Add a new root path (resolves `~` and relative paths) |
| `/api/roots` | `DELETE` | Remove/forget a root path (`?path=...`) |
| `/api/control` | `GET` | Get pause/resume state |
| `/api/control/pause` | `POST` | Pause indexing operations |
| `/api/control/resume` | `POST` | Resume indexing operations |
| `/api/control/sync` | `POST` | Trigger index synchronization |
| `/api/stats` | `GET` | Document and root summary counts |
| `/api/scan/live` | `GET` | Active discovery and walk progress |

## Integration Status and Pending Gates

- **Unit & Boundary Tests**: 100% green on macOS darwin/arm64 covering path sandboxing, symlink escape rejection, CSRF protection, SSE streaming, envelope parsing, and root management.
- **Pending Gates**: Real-browser UAT will be performed by the manager after the backend search index commands (`search --indexed`, `search index status`, `add`, `forget`, `pause`, `resume`) are connected.
