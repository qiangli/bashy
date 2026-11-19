---
id: 5b004778-4ac2-4672-88c6-5c41b0b1b12b
seq: 30
form: page
type: runbook
title: 'Shaping-enable an existing website (static host + your shaping host)'
description: Make an existing static site (GitHub Pages or any host) shapeable for free with website.bsh — visitors by tier (anonymous, signed-in, circle member, owner), private fields by audience, generated views — and the pitfalls a live run hit — WHEN a user wants their site, blog, résumé or vlog to be shapeable, show private details only to chosen circles, or add a Shape button (keywords website, static site, github pages, shaping layer, website.bsh, engine face, audience, circle, recruiter, panel, Shape button, static_only, sign in, satellite).
tags:
    - shaping
    - website
    - outpost
    - satellite
status: candidate
evidence: 'Live run on a Next.js static export behind a satellite and a Windows owner outpost: tier matrix, fail-closed fields, generated views, offline page, owner Shape page. Pitfalls below were each hit and fixed.'
created: "2026-11-19T04:40:40Z"
updated: "2026-11-19T04:40:40Z"
---

The site stays where it is. A small **shaping layer** (its own shaped app, in a PRIVATE companion repo) fronts it through the owner's outpost as a cloudbox-public app at `https://SATELLITE/matrix/h/HOST/app/NAME/` (the **engine face**), injects a panel, and serves each visitor by tier: anonymous (the page as is), signed-in (views, a pseudonym — never their email), circle member (plus fields whose audience admits their circle), owner (everything, plus "Shape this site").

1. `bashy src fetch bashy`; the commands are `examples/shaping/website/website.bsh` and `examples/shaping-host/shaping-host.bsh` (run with `bashy --bashsharp`; each has `--help` and a skill face).
2. `website.bsh assess https://SITE` → `website.bsh enable DIR --site https://SITE --title "NAME"` into a new private repo. Edit `site.json` (AI route, views, `static_only`, `shape_url`) and `fields/*.md` (one private fact each; front matter `circles:`/`groups:`/`emails:`/`signed_in:`; no audience = owner only).
3. Host it on the owner's always-on outpost (`shaping-host.bsh up`, then `website.bsh serve DIR --name NAME`) or, session-scoped, on a free cloud workspace (the `.devcontainer` that `enable` writes). The host must be attached to the owner's satellite; that is where the engine face lives.
4. Put one link on the static site to the engine face (the Software Shaping Flow mark as the icon), and keep private details off the static pages: mark the contact block `data-shaping-contact`, its sign-in link `data-shaping-signin`, and engine-only elements `data-shaping-hide-on-engine`.
5. Owner shaping: `website.bsh register DIR --name NAME` makes the layer a shaped app in the console; set `shape_url` so the panel offers the owner "Shape this site" (see the shaping-agent runbook).

Pitfalls (each seen live):
- **Sign-in is the satellite's root page** (`/`, provider buttons); `/signin` is a 404 there, and a session on the master does not carry to the satellite yet — sign in on the satellite.
- **A signed-in non-member is anonymous on a satellite** today (admission is members-only). Members and the owner get their tiers.
- **Never proxy a site's assets through the engine**: the edge rate-limits anonymous requests per app, and a page of bundles trips it (HTTP 429). The template loads assets from the static host and only pages through the engine.
- **App-router sites (Next.js, SPAs) show their 404 on the engine face** after hydration (the mount path is not one of their routes): set `"static_only": true`; the panel then fills the page hooks.
- The panel only appears where visitors can be identified; inside the console copy it stays hidden — use the console's own wish button there.
