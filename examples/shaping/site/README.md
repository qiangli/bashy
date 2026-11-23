# examples/shaping/site — generic `<owner>/site` content module and shaped app

Every site owner gets a public or private `<owner>/site` Go module generated
from one bashy template. The module root is importable `package site` (so
another Go service can import `<module>` at a pinned version to load, render,
negotiate, or mount its routes), while `cmd/server/main.go` builds the
standalone HTTP server and check CLI (`shaped.json` `server.main: "cmd/server"`).

One Markdown file per page (`content/*.md`, with declared `{#id}` section
identifiers — never derived from headings) renders deterministically into
server-rendered HTML, Markdown and JSON under one short SHA-256 content
`revision`:

| Representation | Negotiated (`Accept`) | Direct twin | Carries |
|---|---|---|---|
| HTML | default (`text/html`, `*/*`) | `/` · `/about` | `<section id="...">`, optional lead `<p>` summary under `<h1>`, shared `<nav>`, `<link rel="canonical">`, `<link rel="alternate">`, `<meta name="revision">` |
| Markdown | `Accept: text/markdown` | `/index.md` · `/about.md` | front matter (`path`, `title`, `revision`, `alternates`), labeled nav links, optional lead summary paragraph under `# Title`, `## Heading {#id}` |
| JSON | `Accept: application/json` | `/index.json` · `/about.json` | `{path, title, summary?, revision, nav, sections: [{id, heading, html, markdown, sha256}]}` |

Every page and twin response sends `Vary: Accept`, a strong `ETag` equal to
`"<revision>"`, and `Link` `rel="alternate"` headers for both twins, alongside
`/robots.txt` and `/llms.txt`.

## Front matter

Each `content/*.md` page begins with a `---` front matter block:

- `title` (required) — page title.
- `path` (required) — canonical URL path starting with `/` (e.g. `/` or `/about`).
- `summary` (optional) — one lead paragraph exposed as `Page.Summary` (`"summary"` in JSON when non-empty, `<p>...</p>` under `<h1>` in HTML, a paragraph under `# Title` in Markdown, and part of `CanonicalDocument` so editing it changes `Page.Revision`).
- `sections` (optional) — comma-separated list asserted against body `{#id}` order.
- Unknown `key: value` lines in front matter are ignored; malformed lines missing `:` are rejected.

## Library API (`package site`)

- Types: `Config`, `NavItem`, `Section`, `Page`, `Site`, `CheckOutcome`, plus embedded `Assets` (`embed.FS`).
- Loading & validation: `Load(dir)`, `LoadFS(fsys)`, `LoadEmbedded()`, `Validate(site)`.
- Rendering & twins: `RenderHTMLFragment` / `(*Page).RenderHTMLFragment`, `RenderMarkdown` / `(*Page).RenderMarkdown`, `RenderJSON` / `(*Page).RenderJSON`, `(*Site).RenderHTMLDocument`, `(*Site).RenderRobotsTxt`, `(*Site).RenderLLMsTxt`, `TwinPaths`.
- Hashing: `CanonicalDocument`, `ComputeRevision`, `ComputeSectionHash`.
- HTTP negotiation & routes: `NegotiateAccept`, `NegotiateRequest`, `NewHandler`, `NewHandlerFS`, `NewServerHandler`.

## CLI usage

```bash
bashy --bashsharp site.bsh init ./my-site --title "Example Site" --origin https://example.com --owner acme
# or with an explicit Go module path:
bashy --bashsharp site.bsh init ./my-site --title "Example Site" --origin https://example.com --module example.com/acme/site
bashy --bashsharp site.bsh render ./my-site --format html /about
bashy --bashsharp site.bsh render ./my-site --format md /about
bashy --bashsharp site.bsh render ./my-site --format json /about
bashy --bashsharp site.bsh check ./my-site
bashy --bashsharp site.bsh serve ./my-site --port 18940
```

`site init` writes `go.mod` with `--module PATH` (defaulting to
`github.com/<owner>/site` when `--owner` is given; required otherwise) and
rewrites internal imports in `cmd/server/main.go`. `template/go.mod` stays
`module site` for standalone template tests. All verbs support `--dry-run` and
`--json`.

## Shaping and skill catalog

- **Shaped-app face**: `init` writes `shaped.json` (`server.main: "cmd/server"`),
  `contract.bsh` and four frozen checks under `checks/`:
  - `render-determinism.bsh` — rendering twice is byte-identical across HTML,
    Markdown, JSON, `/robots.txt` and `/llms.txt`, and all three representations
    list the same links and section ids.
  - `unique-ids.bsh` — page paths and declared section ids are unique.
  - `links-resolve.bsh` — internal links and `#fragment` section anchors
    resolve.
  - `stale-base.bsh` — compare-and-set on `bashy shape wish --element
    'section=ID revision=REV'`: refuses stale revisions with `stale, re-read`.
- **Per-instance skill**: `init` emits `SKILL.md` and
  `skills/<owner>-site/SKILL.md` documenting `Accept` negotiation, twins, and
  the citation record `{url, section_id, revision, section_sha256, quote}`.
  Admit it to the local catalog with `bashy skill add ./my-site/skills/<owner>-site`.

## Optional DNS TXT

Publish an optional DNS TXT record at `_agents.<host>` so agents can discover
the site without fetching it first:

```text
_agents.<host>  TXT  "v=agents1; index=https://<host>/agents.txt; mcp=https://<host>/mcp"
```

The record is advisory only; `/agents.txt` remains the canonical entry point.

## Tests

- `GOWORK=off go -C template test ./...` — stdlib-only `package site` library
  and `./cmd/server`, `summary:` front matter, revision invariance, `Accept`
  negotiation, twins, headers, `/robots.txt`, `/llms.txt`, representation
  parity, and all four checks.
- `GOWORK=off go test ./...` — `site.bsh` verbs (`--dry-run` and `--json`),
  owner module generation (`github.com/<owner>/site` and `--module`), external
  Go module `import` via `replace`, per-instance skill admission, and
  `bashy shape verify` in the verifier's scrubbed environment.
