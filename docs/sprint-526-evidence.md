# Sprint 526 B1b: desk notepad and form

Story #2969 (`8f71feadf6cc`). The two shipped `register.sh` scripts now use
`bashy app add NAME --type shaped --source DIR`, the same public command an
external app uses. Their `shaped.json` files provide the server entry points.
The command vendors each source into Bashy's managed checkout; no builtin app
route or privileged registration was added. The shaped app UI can therefore use
the public per-principal revision, history, and Genie preview flow for both.

Focused red/green proof: `go test ./examples/desk -run
'^TestDeskRegisterScriptsUsePublicShapedAppPath$' -count=1 -v`.

- Red, before the installer change: notepad and form both reported `Kind:app`
  with no shaped source.
- Green: the test runs each shipped installer in an isolated home, checks
  `app show --json` for `type: shaped` and its managed source, checks `app list`
  for the registered row, then forks, verifies, activates, reads history, and
  undoes a personal revision as `device:alice` for each app. Undo returns from
  the second activated revision to the first.

Green result: `PASS` (165.22s); notepad 64.17s, form 65.87s.

The existing `TestDeskToolsShipShapedServer` covers manifest parsing and the
server build/health probe independently. No app-specific extension was needed.
