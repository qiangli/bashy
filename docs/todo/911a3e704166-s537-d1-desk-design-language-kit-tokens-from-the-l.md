---
id: 911a3e704166
kind: feature
title: 'S537.D1 desk design language: kit tokens from the launcher design system, one app shell, kit rendering, conformance check'
seq: 2964
status: done
priority: p1
labels:
    - desk
created: 2026-10-30T18:44:08.321779Z
weave: 366
assignee: claude-opus5.5
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T19:09:32.288891Z
closed_by: s537-manager
---

Operator-directed (2026-10-30). Principle: "bashy is one tool ... cohesive". Desk tools are parts of bashy and use its conventions; where the kit differs from bashy, the kit changes. Amends R1's "no user-visible change" for LOOK only; behaviour unchanged.
Directive (verbatim): "now we have whiteboard, notepad, viewer. ensure that they are consistent in rendering, processing so they will act/look like from the same dest/suite ... e,g font family/size background etc. they are distinctly different but yet naturally fitting with each other"
Today: four palettes — launcher yoke/pkg/webconsole/artifact/app.css (gray + #2563eb), kit.css (cream + teal #1f5c53), chart (hardcoded navy), whiteboard/brainstorm index.html (no kit.css, own colours).
Scope:
1. Tokens in examples/desk/kit/kit.css derived from the launcher design system: font stack + type scale, spacing, radius, shadow, neutral surfaces, light/dark — all CSS variables. Family accent palette: one accent + icon per tool (whiteboard, brainstorm, notepad, viewer, form, chart), harmonised with launcher primary.
2. One app shell for every tool: top bar (icon + name, item id, status badge, share/copy, theme toggle) in the same place; same empty/error/loading states, shortcuts, toasts.
3. Rendering: all text content through the kit renderer + kit content styles (notepad preview, viewer, form help, brainstorm cards, whiteboard text, chart labels/tables). Chart and ```chart embed use the kit data-colour palette. N3 renderer emits semantic classes only; D1 styles them.
4. Retrofit chart, form, notepad, whiteboard, brainstorm onto the shell in this story.
5. contracttest kit-conformance: page loads kit.css + shell; no hex colours or font-family in tool CSS outside the kit (allowlist user-content colours: whiteboard pens, chart series data).
6. Evidence: W5 captures light + dark screenshot of every tool into one side-by-side gallery (viewer can host).
Acceptance: contracttest conformance red on a planted hex colour/font-family, green on all tools; per-tool tests green; screenshots light+dark.
