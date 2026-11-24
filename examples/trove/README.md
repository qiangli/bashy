# Trove — Virtual Drive Example (`bashy/examples/trove/`)

`trove` is the virtual drive of every cloudbox user: one small path-keyed JSON
document (`bashy-trove-v1`) of mount points with no storage of its own.

## Layout (`bashy-trove-v1`)

```json
{
  "schema_version": "bashy-trove-v1",
  "/home":  {"locator": "dir:home@studio-host"},
  "/drive": {"locator": "drive:My Drive/bashy", "account": "me@example.com"},
  "/repo":  {"locator": "gh:owner/repo",        "account": "owner"}
}
```

- Keys are virtual paths (`/home`, `/drive`, `/repo`).
- Values reuse the `bashy lens` locator grammar (`yoke/pkg/ref/locator.go`).
- `account` names the vault entry holding the credential for cloud/API mounts.
- Stored in the cloudbox vault entry `trove/layout` via the `bashy secret`
  client (`secret exec` / `secret set`), with `--layout FILE` or
  `TROVE_LAYOUT_FILE` as a local override for tests and offline outposts.

## Registration

```sh
./register.sh
```

This registers `bashy trove` and installs its embedded `trove` skill.

## Verbs

- `trove mount add PATH LOCATOR [account=ACCOUNT] [--dry-run] [--json]`
- `trove mount ls [--json]`
- `trove mount rm PATH [--dry-run] [--json]`
- `trove ls [PATH] [--json]`
- `trove find [PATH] [--depth N] [--name GLOB] [--json]`
- `trove stat PATH [--json]`
- `trove cat PATH [--json]`
- `trove put SRC PATH [--dry-run] [--json]`
- `trove add URL PATH [--dry-run] [--json]`

Supported mount schemes in this slice: `dir:`, `dir:@host` (outpost SFTP), and
`git:`. `drive:` and `gh:` mounts can be added to the layout and return exit 3
until H6b lands.

Exit codes: `0` done, `3` venue/availability refusal (peer host offline by
name, read-only mount, deferred scheme), `1` error (not found, invalid input).

## Hermetic tests

```sh
GOWORK=off go -C examples/trove test -v ./...
```
