# Matrix chat (community example)

Matrix chat is a community example, not part of bashy core. Bashy's own chat
is Klatch (`examples/klatch`). This directory is self-contained: `register.sh`
registers the `matrix-chat` command and skill, the `tuwunel` tool declaration
(`tuwunel.yaml`, via `bashy tool add -f`) and the `matrix-chat` service unit
(`matrix-chat.yaml`, via `bashy service add -f`).

This optional command uses Bashy's signed `tuwunel` 1.9.4-bashy.2 tool, the
shipped `matrix-chat` service unit, and the existing secret vault. It does not
use the feed gateway, brainstorm, or whiteboard. On Linux or macOS, register
with `bash examples/matrix-chat/register.sh`, then run `bashy matrix-chat setup
CONFIG.json`. No server is installed or started by registration.

Example config (keep the actual file outside the repository):

```json
{
  "server_name": "chat.example.test",
  "data_dir": "/absolute/private/path/matrix-chat",
  "bind_address": "127.0.0.1",
  "port": 6167,
  "users": [
    {"localpart": "alice", "password_secret": "matrix/alice/password"},
    {"localpart": "bob", "password_secret": "matrix/bob/password"}
  ],
  "rooms": [
    {"key": "project", "name": "Project", "creator": "alice", "invitees": ["bob"]}
  ]
}
```

Create each secret with `bashy secret gen | bashy secret set REF`, or use
`bashy ask` piped into `bashy secret set REF` for chosen passwords. The first
user is made server admin on first setup. The config contains only vault
references; no roster or classroom assumptions are built in. The integration
lane supplies stable Cloudbox subject → Matrix `localpart` mappings and secret
references, then requests `setup` or `provision` with this JSON document.
The command returns Matrix user IDs and keyed room IDs as JSON. An existing
room is found through its stable Matrix alias, so reruns do not duplicate it.

Choose `server_name` before the first setup. It becomes part of every Matrix
ID and is recorded at `data_dir/server-name`; setup refuses a different name
or an unmarked existing database. Registration and federation stay disabled.
The default listener is loopback. To expose chat to another network, use a
separate reviewed reverse proxy and TLS plan; keep the server itself private.

`bashy matrix-chat status|start|stop CONFIG.json` operate the independent
service. `bashy matrix-chat backup CONFIG.json` stops the service, copies the
quiescent database and identity marker to a timestamped sibling directory,
and checks the marker. The service remains stopped after backup; start it
explicitly. To restore, stop the service, back up the current data, verify the
backup marker equals the intended `server_name`, copy the entire backup into a
new empty data directory on the same host, point the config at that directory,
then start and verify login and room history. Never merge two RocksDB trees or
copy one while it is running. Restore is intentionally manual so an existing
database cannot be overwritten by a command typo.

The offline tuwunel admin console requires passwords in its `--execute`
arguments. Setup keeps that process short and suppresses its output, but a
local process observer may see those arguments while provisioning runs. Matrix
logins send passwords in HTTP request bodies to the loopback listener; tokens
are kept in memory. The setup command does not print either.

Run the live test on the manager supplied native host:

```sh
python3 examples/matrix-chat/e2e.py
```

It requires Bashy, signed tuwunel admission, working vault access, and a free
local port. Missing dependencies fail the test. It creates isolated service
state and data, exercises two real Matrix accounts, room invite/join, text and
image messages, a second setup, and stop/restart persistence. It does not use
containers or a mock homeserver.
