# Matrix chat (independent Bashy tool)

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

## Cloudbox accounts (S513.4a)

Cloudbox (`docs/matrix-chat.md` in the cloudbox repo) gives ordinary Cloudbox
users accounts on this homeserver. Cloudbox decides access; this host creates
the accounts. Add a `cloudbox` section to the config (all keys optional):

```json
"cloudbox": {
  "port": 6168,
  "tunnel_token_secret": "matrix-chat/cloudbox/tunnel-token",
  "appservice_token_secret": "matrix-chat/appservice/as-token",
  "homeserver_token_secret": "matrix-chat/appservice/hs-token",
  "account_secret_prefix": "matrix-chat/accounts"
}
```

Bind the host's outpost tunnel secret (Cloudbox `MATRIX_TOKEN`) once with
`bashy ask --name MATRIX_TOKEN --stdout | bashy secret set
matrix-chat/cloudbox/tunnel-token`. Then, after `setup`, run `bashy
matrix-chat cloudbox CONFIG.json`. It:

- generates the appservice tokens in the vault when absent;
- writes `data_dir/cloudbox-appservice.toml`, which declares a tuwunel
  application service with the exclusive namespace
  `@cb_[0-9a-f]{32}:<server_name>`, and points the `matrix-chat` unit at it.
  The unit names vault references, but the service monitor needs host bindings
  for them. Bind `MATRIX_CHAT_APPSERVICE_AS_TOKEN` to
  `@matrix-chat/appservice/as-token` and `MATRIX_CHAT_APPSERVICE_HS_TOKEN` to
  `@matrix-chat/appservice/hs-token` in `~/.config/bashy/secrets.map` (mode
  `0600`) before running `matrix-chat cloudbox`. The shim also needs
  `MATRIX_CHAT_CLOUDBOX_TUNNEL_TOKEN=@matrix-chat/cloudbox/tunnel-token`.
  These are references, not secret values. The monitor can alternatively
  receive the declared unit environment names through its own environment;
- declares the separate `matrix-chat-cloudbox` unit (`cloudbox_shim.py`) on
  loopback `port`, exposed as the outpost app `matrix-chat` with
  `require_login: false`, enables both units and restarts them.

The appservice can create and act for `@cb_…` accounts only. It is not a
server admin. No admin password is shared, and registration stays disabled.
The homeserver keeps its own unit and lifecycle. Stopping the shim
disconnects Cloudbox but does not touch the database. Remove the
`matrix-chat-cloudbox` unit to detach Cloudbox completely.

The shim serves one Cloudbox request. Every other request is forwarded
unchanged to tuwunel when it is under `/_matrix/client/`, `/_matrix/media/`
or `/.well-known/matrix/`; anything else (federation, admin, `/_tuwunel/`,
dot segments) answers 404. Matrix tokens are checked by tuwunel itself.
Cloudbox's edge has already applied the share check.

    POST /_cloudbox/v1/account          (below the app mount)
    Authorization: Bearer <host tunnel secret>
    {"localpart":"cb_<32 hex>","display_name":"…","action":"ensure|reset|deactivate"}

| action | account missing | account exists |
|---|---|---|
| `ensure` | create: `{user_id, created:true, password}` | `{user_id, created:false}`, no password |
| `reset` | create: `{user_id, created:true, password}` | rotate: `{user_id, created:false, password}`; all other sessions are logged out |
| `deactivate` | 404 | irreversible: `{user_id, created:false}`; vault entry removed |

`display_name` refreshes the Matrix display name on ensure and reset. It is
cosmetic: once create or reset has minted a password, a display-name failure
is logged and the request still returns that password, so Cloudbox never loses
the one-time password or sees a completed reset as failed. Errors
use the Matrix shape `{errcode, error}`: 401 bad tunnel secret, 400 bad body,
404 unknown account (deactivate), 409 reset/deactivate when the host credential
is missing or stale, 502 homeserver failure, 503 vault unavailable.

The current password of each account is kept at
`<account_secret_prefix>/cb_<32 hex>` in the vault, because the homeserver
requires the current password before a reset or deactivation. Vault names
are limited to 64 characters, so the prefix may be at most 28. The password
is written to the vault before the account is created, so an account never
exists with a password the host has lost. If a user changes their password in
a Matrix client, a reset answers 409. If the homeserver connection drops during
a reset, the shim asks the homeserver which password is live before deciding
whether the reset completed. Closing the short-lived provisioning device is
best effort and never replaces the result. The operator can then set the password
with the offline console. Passwords and tokens are never logged; the shim
logs the action, localpart and caller email.

Gates:

```sh
python3 -m unittest discover -s examples/matrix-chat -p 'test_*.py'
python3 examples/matrix-chat/cloudbox_gate.py           # signed tuwunel + shim, synthetic test vault
MATRIX_CHAT_GATE_VAULT=bashy python3 examples/matrix-chat/cloudbox_gate.py   # production vault
python3 examples/matrix-chat/cloudbox_e2e.py            # live Cloudbox over HTTPS
```

`cloudbox_gate.py` starts the signed tuwunel directly and runs the real
shim. It covers the shim protocol, four accounts, DM and group text and
image, room privacy, reset and deactivation. Its preflight is read-only and
never installs or downloads: it reads the tuwunel path and this platform's
signed asset sha256 from `bashy tool show tuwunel`, and runs the installed
file only if its digest matches. A missing or mismatched binary fails the
gate (install it beforehand with `bashy tool install tuwunel`), and
`MATRIX_CHAT_TUWUNEL` is accepted only when it names that same verified file. By default its account passwords
go to an in-memory adapter that exists only in the test. Every account the run
mints is removed from the vault through `Provisioner.ref` on every exit,
failures included; a credential left behind fails the gate. The production shim
always uses the vault. `cloudbox_e2e.py` is the acceptance gate. It needs a
disposable Cloudbox reached over HTTPS, a chat host prepared with `setup` and
`cloudbox`, and test tokens (listed in its docstring). It covers three users
opting in and seeing their password once, DM and group text and image through
the Cloudbox route, an uninvolved user being denied, and share revocation.
Without those prerequisites it prints `BLOCKED` and exits 2; it never reports
a pass.

## Live test

Run the live test on the manager supplied native host:

```sh
python3 examples/matrix-chat/e2e.py
```

It requires Bashy, signed tuwunel admission, working vault access, and a free
local port. Missing dependencies fail the test. It creates isolated service
state and data, exercises two real Matrix accounts, room invite/join, text and
image messages, a second setup, and stop/restart persistence. It does not use
containers or a mock homeserver.
