# examples/shaping/website — shaping-enable an existing website

The site stays where it is (GitHub Pages, any static host). A small shaping
layer — its own shaped app, kept in a **private companion repo** — fronts it
through outpost as a cloudbox-public app, injects a panel into every page,
and serves visitors by tier:

| Visitor | Gets | The site learns |
|---|---|---|
| anonymous | the page as today, and a sign-in hint | nothing |
| signed-in, not a member | views open to `signed_in` (e.g. a recruiter summary) | a per-site pseudonym, never the email |
| member of a circle | also the fields whose audience admits that circle | email and circle |
| owner | everything; shapes the layer with `bashy shape` | everything |

```bash
bashy --bashsharp website.bsh assess https://example.com
bashy --bashsharp website.bsh enable ~/my-site-shaping --site https://example.com --title "My Name" --contact mailto:me@example.com
# edit site.json (AI route, views) and fields/*.md (private facts + audience)
export SITE_AI_KEY=...            # any OpenAI-compatible route; or point base_url at `bashy llm`
bashy --bashsharp website.bsh serve ~/my-site-shaping --name site
bashy --bashsharp website.bsh register ~/my-site-shaping --name site   # owner shaping
```

On a free Codespace, `enable` has already written `.devcontainer/` and
`bin/{website,shaping-host}.bsh` into the companion repo: push it as a
PRIVATE repo, set the Codespaces secrets `BASHY_PAIR_CODE`,
`SHAPING_SATELLITE` and `SITE_AI_KEY`, and open it in a Codespace; it runs
`shaping-host up` and `website serve` on every start.

Then link the engine face from the static site:
`<a href="https://<cloudbox>/matrix/h/<host>/app/site/">Shape this page</a>`.

## How it works

- `template/` is the shaping layer `enable` copies: a stdlib-only Go server
  (`main.go`, `identity.go`, `audience.go`, `proxy.go`, `view.go`) with
  `site.json`, `fields/` and `public/` embedded, plus `shaped.json`,
  `contract.bsh` and `checks/` so the console can build and shape it.
- **Identity** is outpost's signed relationship stamp (`X-Outpost-Relationship`,
  `-Circle`, `-Groups`, `-Visitor`, `-Relationship-Sig`; see outpost
  `docs/cooperative-web-apps.md`). Unsigned, stale or forged headers are
  anonymous. Outpost never forwards a non-member's address.
- **Audience** per field uses the cloudbox rule grammar plus `signed_in`:
  circles AND (group OR email); a group never widens across circles; a field
  with no audience is owner-only.
- **Views** call one OpenAI-compatible route with only the caller's data
  (public page text + admitted fields). No anonymous model calls; a per-caller
  and a total daily cap protect the owner's free quota.
- **Proxy**: pages come from the static host, root-absolute links are
  rebased under the mount prefix, and the panel is injected before `</body>`.

`GOWORK=off go -C template test ./...` covers the tier matrix, forged stamps,
fail-closed fields, link rebasing and the view's data minimization and caps.
