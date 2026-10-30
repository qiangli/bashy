# Plan — desk family look (S537.D1, story #2964)

Operator directive (2026-10-30): whiteboard, notepad, viewer (and form,
chart, brainstorm) must look and behave like one suite — same font, sizes,
backgrounds, shell — distinct yet naturally fitting. Principle: bashy is one
tool; desk tools use bashy's conventions, and where the kit differed, the
kit changed. Amends R1's "no user-visible change" for LOOK only; behaviour
is unchanged (every tool's Go tests and `browser-test.mjs` stay green).

## Before

Four palettes: launcher (`yoke/pkg/webconsole/artifact/app.css`, zinc +
`#2563eb`), `kit.css` (cream + teal `#1f5c53`), chart (hardcoded navy), and
whiteboard/brainstorm `index.html` (own colours, dark-green hero headers).

## Delivered

1. **Tokens** in `examples/desk/kit/kit.css`, derived from the launcher:
   font stack + type scale, 4px spacing, radius, shadow, zinc neutrals,
   light/dark — all CSS variables. Legacy `--desk-*` names kept. One family
   accent + icon per tool (`kit.Tools`; launcher tile icons match), a
   categorical data palette `--desk-data-1..8` (`kit.DataPalette`), and
   theme-constant "paper" tokens for user-content surfaces (canvas, notes).
2. **App shell** — `kit.Page` + `kit.AppBar`: icon + name, item id, status
   badge, share/copy, theme toggle, same place in every tool; shared
   empty/error/loading states (`kit.State`), toasts (`deskToast`),
   alert kinds, shortcuts (⌘/Ctrl+S, ⌘/Ctrl+Enter, `t`, `y`, `?`), remembered
   theme applied before paint (kit.js moved to `<head>`).
3. **Renderer** — `deskRender` in `kit.js` (safe Markdown subset, DOM-built,
   semantic tags only, ```` ```chart ```` fences on the data palette) styled by
   `.desk-content`. Used by notepad preview (new, live), form help and reply
   reasons, brainstorm cards, whiteboard notes. Chart SVG text/series use
   `--desk-font` / `--desk-data-*` (goldens regenerated). The N3 viewer
   renderer should emit the same semantic tags and reuse `.desk-content`.
4. **Retrofit** chart, form, notepad, whiteboard, brainstorm onto the shell.
5. **Conformance** — `contracttest.StyleViolations` runs inside every
   tool's contract test: page loads kit.css + `.desk-appbar`; no hex/rgb/hsl
   colour or font-family in tool CSS, style attributes, SVG presentation
   attributes or script colour/font assignments outside the kit. Allowlist:
   kit data palette + `Config.AllowColors` (whiteboard pens). Red proven by
   fixture `-break hex-color|font-family|no-shell` and
   `conformance_test.go`; chart goldens checked with `FragmentViolations`.
6. **Evidence** — `examples/desk/gallery/capture.mjs` → `gallery/index.html`
   with light + dark screenshots of every tool side by side (the viewer can
   host it once N3 lands; W5 can extend it).

## Not in this story

- The viewer tool itself (N3): only its accent/icon are reserved here.
- Launcher tiles for form (G1).
