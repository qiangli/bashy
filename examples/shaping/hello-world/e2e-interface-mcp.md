# Selected interface: MCP

Discover tools from the operator-configured candidate Bashy MCP server. The
candidate exposes the `shape` tool with `args` as its argument vector; confirm
this against actual tools/list before invoking it. Call that tool for every
shaping operation. A typical arguments object is
`{"args":["status","hello-world","--json"]}`. Do not substitute shell CLI
calls or raw HTTP for MCP calls. Preserve actual tools/list and tools/call
events, server identity and sanitized results. Missing tools or denied effects
are BLOCKED; do not switch interfaces to complete this cell.


The operator must grant the shaping tool's declared effects when starting the
isolated test server: `bashy mcp serve --tools shape --allow destroy,spend`.
The tool carries aggregate command effects, so its default capability guard also
refuses read-only invocations such as help/status without those grants. Configure
this on the test server, not as extra arguments to `shape`. Keep the disposable
installation and normal actor checks from the run context.
