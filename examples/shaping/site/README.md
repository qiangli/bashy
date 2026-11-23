# examples/shaping/site — generic `<owner>/site` content module and shaped app

Every site owner gets a public or private `<owner>/site` repository generated
from one bashy template. One Markdown file per page (`content/*.md`, with
declared `{#id}` section identifiers — never derived from headings) renders
deterministically into server-rendered HTML, Markdown and JSON under one short
SHA-256 content `revision`:

| Representation | Negotiated (`Accept`) | Direct twin | Carries |
|---|---|---|---|
| HTML | default (`text/html`, `*/*`) | `/` · `/about` | `<section id="...">`, shared `<nav>`, `<link rel="canonical">`, `<link rel="alternate">`, `<meta name="revision">` |
| Markdown | `Accept: text/markdown` | `/index.md` · `/about.md` | front matter (`path`, `title`, `revision`, `alternates`), labeled nav links, `## Heading {#id}` |
| JSON | `Accept: application/json` | `/index.json` · `/about.json` | `{path, title, revision, nav, sections: [{id, heading, html, markdown, sha256}]}` |

Every page and twin response sends `Vary: Accept`, a strong `ETag` equal to
`"<revision>"`, and `Link` `rel="alternate"` headers for both twins, alongside
`/robots.txt` and `/llms.txt`.

```bash
bashy --bashsharp site.bsh init ~/my-site --title "Example Site" --origin https://example.com
bashy --bashsharp site.bsh render ~/my-site --format html /about
bashy --bashsharp site.bsh render ~/my-site --format md /about
bashy --bashsharp site.bsh render ~/my-site --format json /about
bashy --bashsharp site.bsh check ~/my-site
bashy --bashsharp site.bsh serve ~/my-site --port 18940
```

All verbs support `--dry-run` and `--json`.

## Shaping and skill catalog

- **Shaped-app face**: `init` writes `shaped.json`, `contract.bsh` and four
  frozen checks under `checks/`:
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
  Admit it to the local catalog with `bashy skill add ~/my-site/skills/<owner>-site`.

## Tests

- `GOWORK=off go -C template test ./...` — stdlib-only renderer, revision
  invariance, `Accept` negotiation, twins, headers, `/robots.txt`, `/llms.txt`,
  representation parity, and all four checks.
- `GOWORK=off go test ./...` — `site.bsh` verbs (`--dry-run` and `--json`),
  per-instance skill admission, and `bashy shape verify` in the verifier's
  scrubbed environment.
