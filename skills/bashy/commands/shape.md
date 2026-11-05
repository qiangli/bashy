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

## Remote console

`--remote URL --cookie-file FILE` uses an existing paired console session.
FILE contains only the value of the `bashy_console` cookie, never an HTTP header.
Use HTTPS outside loopback. Session cookies are never placed in URLs or error
messages, and redirects are refused. The server resolves caller and principal;
`--user`, `--principal` and `--as` cannot override remote identity.

The remote adapter supports the existing status, wish, show, diff, verify,
approve, reject, suggest, history, undo, restore and incident report/show endpoints. Local workspace
import, incident listing and source operations lacking console endpoints return an explicit
unsupported-operation error. Remote wish retains the web API's asynchronous
candidate response; inspect status/show until generation finishes.
