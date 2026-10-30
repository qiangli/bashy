# Plan — desk viewer (S537.V1)

Story fc5033e93edd, Sprint 537. Operator scope add 2026-10-30 ("A, add both
to 537 now"), clarified the same day: data separate from template, the agent
never generates HTML. Built after D1 (kit tokens + app shell), D2 (standard
subverbs + envelope) and N3 (`desk-kit/render`) merged.

## Decisions

- **View-only by construction.** Verbs `serve add show list open share close
  append export join` (kit `ToolVerbs("viewer")`); `set`/`edit`/`put`/`new`/
  `get` exit 2 / HTTP 400. Pages are append-only; a correction is the next page.
- **Item = book.** Ordered pages per sprint (`sprint-N`, stable across manager
  handoffs) or per session (`session-S`). One JSON file per book; per-page asset
  dirs so a later upload never rewrites history.
- **Pipeline.** Data JSON → schema check → logic-light `text/template` with
  escaped values (`mdText` / `mdProse` Stringers) → Markdown → `render.Markdown`
  → kit shell (`kit.Page` + `kit.AppBar`). Live source `x-source: sprint` reads
  `bashy sprint show N --json` server-side (fixed argv, integer arg).
- **Changed since previous.** Block-level multiset diff against the previous page
  of the same kind; per-block rendering for in-place highlight; whole-document
  render for footnotes/TOC; no in-place highlight when everything is new.
- **Export.** Self-contained HTML (kit CSS/JS inline, assets as data: URIs);
  PDF via the page's Print button (human) or headless Chrome (agent:
  `export --to pdf`). `bashy browser` lacks print-to-PDF — follow-up.
- **contracttest.ViewOnly** (additive): set must be 400 before and after close;
  the CLI must refuse `set`/`edit` with exit 2.
- Storage flag is `--dir` (not `-data`) because `--data` is the template data.

## Red/green

Contract red under `-break accept-set|open-emits-content|allow-secret`; the
editing (non-view-only) contract is red against the viewer; data
neutralisation red under `no-escape`; bad-data rejection red under
`no-validate`. `browser-test.mjs` drives the page in Chrome (navigation,
keyboard, highlight, theme, copy, offline export, no script execution).

## Follow-ups (not V1)

- G1 documents notepad (edit) vs viewer (read) and the chat summary + link rule.
- W5 per-OS round-trip includes viewer.
- Chart fences: zero baseline and duplicate caption come from the kit renderer's
  built-in chart (N3 / C1 wiring), not the viewer.
- Gallery capture (`examples/desk/gallery/capture.mjs`) does not yet include viewer.
