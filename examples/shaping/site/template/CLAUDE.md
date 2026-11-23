# CLAUDE.md

Content and shaping rules for this `<owner>/site` module:

- **Content (`content/*.md`, `site.json`).** Every page has front matter with `title` and `path` (starting with `/`), and every `##` section heading carries an explicit `{#id}` (for example `## Team {#team}`). Section ids are never derived from headings.
- **One source, three representations.** The stdlib Go renderer (`render.go`, `server.go`) turns `content/*.md` and `site.json` into HTML, Markdown, and JSON under one canonical content SHA-256 `revision`.
- **Wish provenance (`.wish`).** When a shaping wish's `<<<CONTEXT` includes `element` with `section=<id> revision=<rev>` (and optional `page`), write `.wish` at the repository root verbatim (never substitute the file's revision for the user's `revision=<rev>`):
  ```
  page: <page>
  element: <element>
  request: <one-line summary>
  ```
  When a wish has no `element`, remove `.wish` if present. The frozen `checks/stale-base.bsh` check compares `revision=<rev>` against `HEAD~1` and refuses a stale revision with `stale, re-read`.
- **Checks.** Do not edit `contract.bsh` or `checks/*.bsh`; the verifier runs the base commit's frozen checks in a scrubbed environment.
