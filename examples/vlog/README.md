# examples/vlog — a video blog you author and host yourself

Write a post, drop in a video, get a small static site, and share it two
ways: from your own computer through the bashy console and Outpost (a link
anyone can open), or on GitHub Pages. Pair it with
[examples/video](../video) to make the videos.

```bash
bashy dag examples/vlog build                         # sample/ -> sample/public/
bashy dag examples/vlog serve                         # console tile: http://127.0.0.1:22749/vlog/
bashy dag examples/vlog publish-outpost NAME=vlog     # share it publicly via this host's Outpost
bashy dag examples/vlog publish-pages REPO=you/vlog   # or GitHub Pages
```

Use your own site with `SITE=$PWD/my-vlog`. The same commands work on
macOS, Linux and Windows.

## A site

```
my-vlog/
  site.conf                    TITLE=…  TAGLINE=…
  posts/2026-10-24-hello.md    one post per file, newest first by name
  media/                       optional extra files copied as-is
```

A post:

```markdown
---
title: Hello from bashy
date: 2026-10-24
video: ../videos/hello.mp4
summary: One line for the index page.
---
Paragraphs separated by blank lines. [Links](https://example.com) work.
```

## How it is shared

- **Your computer.** `serve` registers the built `public/` directory with
  bashy's Apps console (`bashy app add NAME --type site --source DIR`);
  the console serves the directory itself at
  `http://127.0.0.1:22749/NAME/`. If the console is not running, `serve`
  says how to start it — `bashy app serve` — and never starts one in the
  background itself. The console serves the site's live directory: rebuild
  and the next request shows the new bytes.
- **Outpost.** `publish-outpost` makes the site **public**: it sets the
  app's auth to public (anyone with the link opens the console path with
  no login) and registers that path with this host's paired Outpost, so
  the link works from anywhere while the bytes come from your machine.
  To undo it: `outpost apps rm NAME`, then
  `bashy app set NAME --auth system`.
- **GitHub Pages.** `publish-pages` pushes `public/` to a branch (default
  `gh-pages`); turn on Pages for that branch once in the repository settings.

## Who does what

An agent writes posts, builds, serves and publishes with the commands above.
A person asks in chat ("post today's demo video"), reviews the page, and
decides who may see it.

## Notes

- bashy serves the site itself (`app add --type site`): the example ships
  no server of its own — nothing to build, no cross-build, one command on
  every OS.
- Keeping the console running across reboots is the host's service
  manager's job; `bashy app serve` runs it in the foreground.
