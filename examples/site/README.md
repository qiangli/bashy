# examples/site — simple company website and web presence

Author and host a clean, high-performance company website or web presence from
your own machine, edit content via an embedded Sveltia CMS at `/admin` and a visual
drag-and-drop editor, and share it two ways: from your own machine through the bashy
console and Outpost (a public link anyone can open), or on GitHub Pages.

```bash
bashy dag examples/site build                         # sample/ -> sample/public/
bashy dag examples/site serve                         # console tile: http://127.0.0.1:22749/site/
bashy dag examples/site edit                          # Sveltia CMS: http://127.0.0.1:22749/site/admin/
bashy dag examples/site publish-outpost NAME=site     # share publicly via local Outpost bridge
bashy dag examples/site publish-pages REPO=you/site   # or GitHub Pages
```

## Structure of a Site

```
my-site/
  hugo.toml                    Site configuration, title, navigation, company params
  content/
    _index.md                  Home page (hero, tagline, core capabilities)
    about.md                   About the company and leadership
    services.md                Services and offerings
    pricing.md                 Pricing tiers and plans
    contact.md                 Contact details and inquiry channels
  layouts/
    index.html                 Home layout with hero and optional sections
    _default/baseof.html       Base layout template
    _default/single.html       Single page article layout
    partials/                  Reusable components (header, nav, footer, pricing)
  static/
    css/style.css              Clean, responsive, dark-mode aware stylesheet
    admin/index.html           Sveltia CMS (MIT) browser editor
    admin/config.yml           Sveltia CMS collections definition
    admin/sveltia-cms.min.js   Vendored local MIT build of Sveltia CMS
    admin/editor/index.html    Visual drag-and-drop page editor (puck / GrapesJS)
  contract.bsh                 Shaping contract declaring effects and routines
  checks/smoke.bsh             Declared verifier checks for software shaping
```

## Starting a New Site

Initialize a new Hugo company website presence:
```bash
bashy site new my-site --title "Acme Systems"
```
Preconfigured with pages, responsive styles, vendored Sveltia CMS at `/admin`, and a visual editor at `/admin/editor/`.

## Editing Content

- **Sveltia CMS at `/admin`**: Opening `http://127.0.0.1:22749/site/admin/` provides
  a lightweight, Git-backed content management SPA (MIT license). Content changes
  are committed directly to Markdown files with front matter.
- **Visual Page Editor**: Opening `http://127.0.0.1:22749/site/admin/editor/` provides
  a visual block builder (compatible with Puck / GrapesJS) for dragging and dropping
  sections (Hero, Capabilities, Pricing, Testimonials, Contact) onto the canvas.

## Serving and Sharing

- **Your machine**: `serve` registers the built `public/` directory with bashy's
  Apps console (`bashy app add NAME --type site --source DIR`); the console
  serves the directory at `http://127.0.0.1:22749/NAME/`.
- **Outpost**: `publish-outpost` sets the app auth to public and registers the
  path with the host's Outpost bridge, providing a link anyone can access while
  content is served directly from your machine.
- **GitHub Pages**: `publish-pages` pushes `public/` to the repository's
  `gh-pages` branch.

## Software Shaping

Every site earns the **Shapeable** badge:
- **Registration**: Registered as a shaped app (`bashy app add site --type shaped --source DIR`).
- **Declared Checks**: Ships `checks/smoke.bsh` to verify build outputs, HTML integrity,
  Sveltia CMS existence, and visual editor presence.
- **Scoped Activation**: A change is active only for the requesting principal
  (`--principal device:alice`). Another principal continues to see their own version.
- **Undo**: Any shaped change can be reverted in one step (`bashy shape undo site`).

## Verified Foundations & License Audit

Every component used adheres strictly to the permissive license rule (no open-core,
permissive license verified directly from upstream `LICENSE`, maintained):

| Component | Upstream Repository | License | Stars | Pinned Reference |
|---|---|---|---|---|
| Generator | `gohugoio/hugo` | Apache-2.0 | 90,461 | Tag `v0.169.0` (`1da10fc44701c566b1669cdb2d12638f155138bd`) |
| Admin CMS | `sveltia/sveltia-cms` | MIT | 3,087 | Tag `v0.265.1` (`9eb015aad09fa538b6ab7d032bf348ea1c0bc241`) |
| Visual Editor | `measuredco/puck` | MIT | 13,696 | Tag `v0.24.0` |
| Visual Editor | `GrapesJS/grapesjs` | BSD-3-Clause | 26,404 | Tag `v0.23.9` |
| Chat Generator | `firecrawl/open-lovable` | MIT | 28,907 | Commit `806b8072f28afea11fa2b028cac1449eff74c511` |
