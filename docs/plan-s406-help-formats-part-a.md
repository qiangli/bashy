# S406 / story d6996255d970: help formats, part (a)

Preserve existing help as the classic source. Intercept root front-door help
requests, select explicit format then environment then fleet detection, and
capture classic output from the same executable with classic selection forced.
Convert that text through one renderer (optional long document supported).
Share atlas-to-RegisteredCommand construction and obtain MCP tool objects from
the actual yoke server through an in-memory tools/list session, avoiding a
second schema/annotation implementation. Retain MCP profile selection and add
full definitions to its existing JSON discovery envelope.

First record classic snapshots and failing CLI tests. Implement and commit the
coherent renderer/selection change, then complete golden, detection, MCP parity
and compatibility coverage. Run focused help/commands/MCP package tests, the e2e
dispatch gate, and a Windows cross-build. No skill moves or embed edits.

## Delivered behavior and validation

`bashy help --format skill|classic|mcp CMD` overrides `BASHY_HELP_FORMAT`.
Root `bashy CMD --help` / `-h` chooses skill on fleet tool detection or
`BASHY_AGENTIC=1`, classic otherwise. Builtins, applets and subcommand operands
retain their existing dispatch. Optional long prose is a converter input for
part (b); no skill files or embed directives changed here.

MCP profiles retain their existing selection semantics. To compare a visible
verb's help object with discovery, use `bashy mcp tools --tools CMD --json`
(or `--tools all`) and select the matching item in the new `definitions`
array. Existing `tools` and `registered` name arrays remain intact. Hidden
verbs can render help but remain absent from public MCP discovery.

Validation on 2026-10-09:

- Red first: explicit skill/MCP and environment selection tests failed before
  implementation; three classic snapshots were recorded from that baseline.
- Focused agentos tests (`Help|Commands|CommandFeature|CommandGroup|CommandSynopses|VerbSynopsis|Usage|MCP|AllListedCommands`): PASS, including goldens for
  Cobra sprint and hand-written MCP help, format precedence/detection,
  three classic snapshots under explicit classic and no detected agent,
  applet/builtin exclusions, and live MCP tools/list parity.
- `go test -tags e2e -run '^TestE2EAllListedCommandsDispatch$' ./internal/agentos -count=1 -timeout=6m`: PASS.
- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/bashy`: PASS.

Part (b), skill folding and embed changes, remains intentionally separate.
