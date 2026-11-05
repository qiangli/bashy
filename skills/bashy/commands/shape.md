---
name: shape
description: Change an application's behavior for a selected principal through isolated candidates, trusted verification, explicit activation and reversible versions.
---

# Shape an app

`bashy shape` is the command-line front door to the same shaping lifecycle as
the app console. Local operations call the shared service directly. App source
comes from the managed checkout imported by `bashy app add --type shaped`.

Choose the represented user with `--user` (`--principal` alias), or
`BASHY_PRINCIPAL`. `--as` identifies the accountable caller and is checked by
the existing session identity resolver. The generating `--agent tool:model`
is provenance separate from the caller and represented user.

```sh
bashy shape status hello-world --user PRINCIPAL
bashy shape wish hello-world 'Change the greeting to Hello, Shaping!' --user PRINCIPAL --agent TOOL:MODEL
bashy shape show hello-world CANDIDATE --user PRINCIPAL --json
bashy shape diff hello-world CANDIDATE --user PRINCIPAL
bashy shape verify hello-world CANDIDATE --user PRINCIPAL
bashy shape preview hello-world CANDIDATE --user PRINCIPAL
bashy shape approve hello-world CANDIDATE --user PRINCIPAL
bashy shape undo hello-world --user PRINCIPAL
```

Local `wish` waits for generation. `wish APP --from WORKSPACE` copies a hand-made
change into an isolated candidate; it does not build in the supplied checkout.
Use mutually exclusive `--story` or `--todo` to link work; otherwise an ordinary
todo is created automatically. Verification failure is a nonzero result with
an inspectable verdict. Approval re-verifies and uses the sealed activation gate.

Web `preview` prints a URL on the paired console (`--console` selects its base
URL). Open it using the browser paired as the same principal. Printing the URL
is not evidence that the page rendered or that a browser assertion passed.
`suggest APP` reads the same active-version suggestions as the web UI.
`incident APP` lists incidents, `incident APP --id ID` inspects one, and
`incident APP MESSAGE --kind script --path /` reports a failure. Reporting
uses the existing incident and repair policy; local repair waits within its
bound instead of abandoning background work when the CLI exits.

History, undo, restore, release, offer and adopt delegate to the existing source
operations and their verification gates. `adopt --preview` does not activate.

## Submit an approved version

`bashy shape submit APP REV [--story ID]` creates a reviewable source proposal
and ordinary weave run through the existing gates. REV is the approved candidate
ID or source revision; the retained approved version remains submittable after
undo. With no `--story`, submission reuses the tracked item and store recorded
with the candidate. An explicit story is resolved in the invocation's ordinary
tracking store. Submission never automatically merges or starts a worker.

## Remote console

`--remote URL --cookie-file FILE` uses an existing paired console session.
FILE contains only the value of the `bashy_console` cookie, never an HTTP header.
Use HTTPS outside loopback. Session cookies are never placed in URLs or error
messages, and redirects are refused. The server resolves caller and principal;
`--user`, `--principal` and `--as` cannot override remote identity.

The remote adapter supports the existing status, wish, show, diff, verify,
approve, reject, suggest, history, undo, restore, submit and incident report/show endpoints. Local workspace
import, incident listing and source operations lacking console endpoints return an explicit
unsupported-operation error. Remote wish retains the web API's asynchronous
candidate response; inspect status/show until generation finishes.

## CLI and service kinds

A trusted app registration may declare `shaping.kind` as `cli` or `service`,
with build/test argument vectors. These policies are held outside candidate
source. Build uses `{output}` and tests use `{artifact}`; arguments are expanded
as vectors without shell interpolation. Web remains the default kind.

For CLI kind, `preview APP ID -- ARGS...` runs the exact verified executable
with bounded process/output capture and a private working directory/home.
With no arguments it tries `--help`; apps without that option can supply
explicit arguments. Nonzero exits remain inspectable failures. Service preview
returns its isolated side-port URL and temporary unit.

CLI/service versions activate for the host. `--user` still identifies the
represented requester and tracking identity; it does not create a private
executable or service instance. Status, history, preview and rollback use that
host scope. CLI binaries occupy immutable side slots while the installed
release and PATH remain intact. Service activation changes only an explicitly
container-backed unit's image, preserving its declared data and other settings.

Host-kind `incident` repair, `offer` and `adopt` are explicitly unsupported in
this MVP. Native errors require kind-aware reproduction before automatic repair;
the web repair path cannot safely stand in for it. CLI registration also needs
an existing released executable for retention; a missing or unproven release
source is never inferred from an imported directory. Source-only
adoption would leave the executable/image inconsistent with history. Web
sharing remains unchanged; kind-aware artifact sharing is follow-up work.
`release` names a known host source revision and records the accountable actor.

For bashy itself, `shape status bashy`, `shape undo bashy`, and
`shape restore bashy REV` always execute the installed recovery implementation,
so those commands do not depend on a healthy shaped CLI. Unknown retained
release source identity is reported as unknown when embedded metadata is absent/dirty
or dependencies came from unversioned workspace replacements. A development
`BASHY_SRC_COMMIT` override remains declared metadata, not proof of the retained
executable's source. Its artifact digest remains
exact. Restoring source/code together may require rebuilding through the frozen
gate and fails explicitly if the required source proof is unavailable.

Authoritative shaping operations and the `src`, app/service and self-management
control paths execute the retained released implementation. MCP server startup
and shell/script entrypoints (`-c`, stdin scripts and interactive use) also stay
released. From a released shell, invoke an explicit `bashy` frontend command to
use a shaped feature. Shaped help and
read-only web-app inspection remain available, while host status/recovery stays
released. A selected image delegates those controls back to the retained release.
This keeps gate routing stable; approved native code still has the local OS
account's authority. It does not sandbox malicious native executables.
