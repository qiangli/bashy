# Selected interface: REST

Use the paired session supplied by file reference in the run context; never
print its contents. Discover the candidate's documented `/api/shape` routes.
Invoke authenticated HTTP requests directly, retaining sanitized method, path,
body, status and response. Do not substitute local CLI or MCP lifecycle calls.
The candidate CLI client's mapping in `shape_remote.go` is a reference for the
current supported subset, not authority to invent missing endpoints.

Use `Accept: application/json` (including diff), and JSON request bodies.
The base is `/api/shape/{app}`: GET the base for status, POST `/candidates` with
`{"request":"..."}`, GET `/candidates/{id}`, GET `/candidates/{id}/diff`,
POST `/candidates/{id}/verify`, POST `/candidates/{id}/approve`, GET `/history`,
POST `/history/undo`, and POST `/submit` with `{"rev":"...","story":"..."}`.
Resolve actual preview through the documented paired browser route; do not
claim an invented REST preview endpoint. Report absent required operations as
BLOCKED. Host-kind mutations require an operator session; a paired user alone
does not authorize self-shaping. Never forge identity/filesystem fields.

