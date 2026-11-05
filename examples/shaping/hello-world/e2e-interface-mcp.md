# Selected interface: MCP

Discover tools from the operator-configured candidate Bashy MCP server. The
candidate exposes the `shape` tool with `args` as its argument vector; confirm
this against actual tools/list before invoking it. Call that tool for every
shaping operation. A typical arguments object is
`{"args":["status","hello-world","--json"]}`. Do not substitute shell CLI
calls or raw HTTP for MCP calls. Preserve actual tools/list and tools/call
events, server identity and sanitized results. Missing tools or denied effects
are BLOCKED; do not switch interfaces to complete this cell.

