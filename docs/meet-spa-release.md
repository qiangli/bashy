# Shipping the meet web room

The meet SPA is **built in CI; its generated `dist/` is not committed**.

The source and lockfile belong to the coreutils sibling at
`../coreutils/pkg/meet/web`, which is pinned by `.sibling-pins`. Building from
that pinned source keeps the UI and the Go embed reproducible and avoids a
second, easily stale copy of generated assets in bashy. Committing `dist` here
is not possible without moving ownership across repositories; committing it in
coreutils would make source changes and generated output two facts that can
drift.

Developer builds are permissive. `make build`, `make build-fips`, `make dist`,
and the `./bashy` bootstrap run `scripts/build-meet-spa.sh optional`. When node
and pnpm (or corepack) exist, the script runs:

```sh
pnpm install --frozen-lockfile
pnpm build
```

and the bashy compilation adds `-tags meetspa`. Without that toolchain it emits
an explicit diagnostic and compiles the existing honest no-UI fallback. It does
not infer success from an old `dist/`.

Releases are fail-closed. The release workflow installs the pinned pnpm
version, GoReleaser runs `scripts/build-meet-spa.sh required`, and every bashy
target is compiled with `-tags meetspa`. A GoReleaser post-build hook inspects
the build metadata of every produced bashy binary. For the native Linux/amd64
artifact in CI (or whichever artifact matches the local host) it additionally
starts `bashy meet service`, fetches `/`, rejects the
fallback text, and requires the served HTML to contain the injected
`<base href="/">`. A failed assertion stops GoReleaser before publication.

Useful local checks:

```sh
make build-bashy
scripts/verify-meet-spa-release.sh bin/bashy darwin_arm64

# Required/release mode must fail when the frontend toolchain is absent.
PATH= scripts/build-meet-spa.sh required
```
