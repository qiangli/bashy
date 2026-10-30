# Desk CLI and API conformance plan

Sprint 537, story 2965. The desk action names are `add`, `show`, `set`,
`list`, `open`, `share`, and `close`, with tool-specific verbs such as
`export`, `append`, and `wait`. Legacy `new`, `get`, and `put` must fail.

The shared desk kit owns the JSON version, envelope, field projection,
common CLI option parser, and MCP help shape. Each tool keeps its domain
storage and item validation. The HTTP adapter applies the shared output
shape to the same action route the CLI calls. The shared contract test
probes both doors, including planted failures. Notepad sends HTML and PDF
imports through `bashy lens`.

Verify with the focused Go suites for the kit and five tools, then run the
repository gate. Update launchers, command registrations, and page scripts
with the standard verbs before the desk guidance story lands.

The viewer story follows the same schema when its tool is added: its
view-only verbs are `add`, `show`, `list`, `open`, `share`, `close`, and
`append`. It has no `set` action. Register its atlas verb rows from the
shared schema generator and run the same contracttest CLI and HTTP probes.
