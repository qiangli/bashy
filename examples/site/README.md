# Site chat template

`site new --template chat [NAME]` installs and registers a shapeable Linux web
app from the exact fork commit below. It creates generated-site previews in
local Podman containers through `bashy oci`; the app itself listens only on
loopback.

Set `OPENAI_COMPATIBLE_BASE_URL` to the owner's gateway or provider. The API
key is optional. Firecrawl cloning is disabled unless the owner explicitly sets
both `ENABLE_FIRECRAWL=true` and `FIRECRAWL_API_KEY`.

```sh
./register.sh
site new --template chat my-site
OPENAI_COMPATIBLE_BASE_URL=http://127.0.0.1:11434/v1 site start my-site
site status my-site
site stop my-site
```

The registered shaped source declares executable checks. Use the ordinary
`bashy shape wish`, `verify`, `preview`, `approve`, and `undo` lifecycle; web
activation is scoped by the console to the requesting principal.

## Provenance

- Upstream: `firecrawl/open-lovable`
- Upstream license read 2026-11-13: MIT
- Upstream stars read from the GitHub repository API 2026-11-13: 28,907
- Fork: `qiangli/open-lovable-shape`
- Exact fork commit: `8544bc18c380e106cf4935c1112461b47e5cbfff`
- Fork branch: `shape/local-owner-runtime`

Targeted source gates: `npm run test:shape`, `npm run build`, and
`script/no-phone-home.sh` (clean). Podman and no-unexpected-egress proof belongs
on the claimed Linux gate host, never on the development box.
