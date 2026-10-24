# examples/vlog — a video blog you author and host yourself

Write a post, drop in a video, get a small static site, and share it two
ways: from your own computer through Outpost (a link anyone can open), or on
GitHub Pages. Pair it with [examples/video](../video) to make the videos.

```bash
bashy dag examples/vlog build                         # sample/ -> sample/public/
bashy dag examples/vlog serve                         # http://127.0.0.1:18090/
bashy dag examples/vlog publish-outpost NAME=vlog     # share via this host's Outpost
bashy dag examples/vlog publish-pages REPO=you/vlog   # or GitHub Pages
```

Use your own site with `SITE=$PWD/my-vlog`.

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

- **Your computer.** `serve` listens on loopback only; `publish-outpost`
  registers it with the host's paired Outpost as a public app, so the link
  works from anywhere while the bytes come from your machine. Outpost apps
  with `--require-login` are limited to you and the people you share with.
- **GitHub Pages.** `publish-pages` pushes `public/` to a branch (default
  `gh-pages`); turn on Pages for that branch once in the repository settings.

## Who does what

An agent writes posts, builds, serves and publishes with the commands above.
A person asks in chat ("post today's demo video"), reviews the page, and
decides who may see it.

## Notes

- `vlog-server` is ~30 lines of Go standard library: static files on
  loopback, `nosniff` and `no-referrer` headers. `server-windows`
  cross-builds it for a Windows host that has no Go toolchain.
- The server runs in the foreground; keeping it running across reboots is
  the host's service manager's job.
