# Klatch setup (Bash# command)

`bashy klatch` bootstraps and provisions a running **Klatch** server (our
rebranded Databag fork). Register with `bash examples/klatch/register.sh`,
then run `bashy klatch setup|provision|status CONFIG.json`. Registration
installs and starts nothing; the server must already be running at `base_url`
(for example `http://127.0.0.1:7100`, or a public proxied app URL).

Example config (keep the real file outside the repository):

```json
{
  "base_url": "http://127.0.0.1:7100",
  "domain": "chat.example.test",
  "admin_password_secret": "klatch/admin",
  "users": [
    {"handle": "alice", "password_secret": "klatch/alice"},
    {"handle": "bob", "password_secret": "klatch/bob"},
    {"handle": "carol", "password_secret": "klatch/carol"}
  ],
  "groups": [
    {"key": "class", "name": "Class", "owner": "alice", "members": ["bob", "carol"]}
  ]
}
```

Create each secret with `bashy secret gen | bashy secret set REF`. Passwords
are only vault references; they are resolved with `bashy secret show` and are
never printed or placed in argv. There is no built-in roster.

| Action | Effect |
| --- | --- |
| `setup` | bootstrap the admin password if the server is fresh, set the federation domain and closed access, then provision |
| `provision` | create missing accounts, mutual contacts (owner and members), and groups |
| `status` | report reachability, bootstrap state, account count, groups found |

`setup` and `provision` print JSON: `{"handles": {handle: guid}, "groups":
{key: id}}`. Reruns skip existing accounts, contacts and groups (groups are
matched by subject), so they never duplicate.

Tests (fake HTTP server, no real server needed):
`python3 -m unittest examples/klatch/test_provision.py`

## Participant CLI verbs

Beyond `setup|provision|status`, `bashy klatch VERB` forwards to the Klatch
`klatch` executable (app/client/cli) and requires **Node >= 22**. Set
`KLATCH_CLI` to its `dist/main.js` (run with `node`) or to an executable;
otherwise `klatch` on PATH is used. Missing CLI or Node reports
`{"ok":false,"error":{"code":"CLI_NOT_FOUND"|"NODE_REQUIRED"}}` on stderr.

Verbs: `auth-login auth-status auth-logout seal-unlock contacts-list
contacts-invite contacts-accept conversations-list conversations-show
conversations-create conversations-rename conversations-leave members-list
members-add members-remove messages-list messages-show messages-send
messages-edit messages-delete attachments-upload attachments-download watch`.

JSON input comes from stdin (default) or `--input FILE|-`; `--session FILE`
and `--json` are accepted. Success prints `{"ok":true,"result":...}`; failure
prints `{"ok":false,"error":{"code":...}}` on stderr. `watch` emits one JSON
object per line. Outgoing assistant text is prefixed `[Assistant: NAME]` by the
CLI. Wrapper tests: `python3 -m unittest examples/klatch/test_cli_wrapper.py`.

Klatch is based on Databag by Roland Osborne
(github.com/balzack/databag), Apache-2.0.
