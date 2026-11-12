# Klatch setup (Bash# command)

`bashy klatch` bootstraps and provisions a running **Klatch** server (our
rebranded Databag fork). Register with `bash examples/klatch/register.sh`,
then run `bashy klatch setup|provision|status CONFIG.json`, and
`bashy klatch sync HOST [--app klatch] CONFIG.json` to push the cloudbox
host-grant roster. Registration
installs and starts nothing; the server must already be running at `base_url`
(for example `http://127.0.0.1:7100`, or a public proxied app URL).

Example config (keep the real file outside the repository):

```json
{
  "base_url": "http://127.0.0.1:7100",
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
never printed or placed in argv. The member roster is not configured here:
`sync` pushes it from the cloudbox host grant.

| Action | Effect |
| --- | --- |
| `setup` | bootstrap the admin password if the server is fresh, set closed access (no federation domain, so external contacts are refused), then provision |
| `provision` | create missing accounts, mutual contacts (owner and members), and groups |
| `status` | report reachability, bootstrap state, account count, groups found |
| `sync` | push the cloudbox host-grant roster to the server's SSO accounts; `--dry-run` prints the roster without sending |

`setup` and `provision` print JSON: `{"handles": {handle: guid}, "groups":
{key: id}}`. Reruns skip existing accounts, contacts and groups (groups are
matched by subject), so they never duplicate.

## Roster sync (host grant)

`bashy klatch sync HOST [--app klatch] CONFIG.json [--dry-run]` reads
`bashy share who HOST/APP --json` — the host grant roster: the owner plus
every member email the grant admits — and PUTs `{"owner": ..., "emails": [...]}`
to `PUT /admin/sso/roster` with the admin password from `admin_password_secret`
(resolved from the vault, never printed or in argv). The server creates,
enables and disables single-sign-on linked accounts to match (password
accounts are never touched) and its JSON response (`members`, `created`,
`enabled`, `disabled`) is printed. The PUT is idempotent: reruns converge with
`created: 0, disabled: 0`. `--dry-run` prints the roster without sending.
`BASHY_KLATCH_WHO_CMD` overrides the who command (used by the tests).

Tests (fake HTTP server, no real server needed):
`python3 -m unittest examples/klatch/test_provision.py`

## Participant commands

`klatch.bsh` is the primary participant command and shaping entrypoint: it
routes verbs, shapes input and composes workflows. The Node helper
(`app/client/cli`, `dist/main.js`, **Node >= 22 required**) only supplies the
protocol, seal crypto, session and file mechanism. Set `KLATCH_HELPER` to the helper's `dist/main.js` (run with `node`) or an executable;
otherwise `klatch-helper` on PATH is used. Missing helper or Node reports
`{"ok":false,"error":{"code":"CLI_NOT_FOUND"|"NODE_REQUIRED"}}` on stderr.

Verbs: `auth-login auth-status auth-logout seal-unlock contacts-list
contacts-invite contacts-accept conversations-list conversations-show
conversations-create conversations-rename conversations-leave members-list
members-add members-remove messages-list messages-show messages-send
messages-edit messages-delete attachments-upload attachments-download watch`,
plus the workflow `post-file`.

JSON input comes from stdin (default) or `--input FILE|-`; `--session FILE`
and `--json` are accepted. As shaping sugar, fields may instead be given as
`key=value` (string) or `key:=JSON` arguments, e.g.
`klatch messages-send channelId=C1 text=hi count:=3`; `KLATCH_ASSISTANT` supplies
the default `assistant` (also inside stdin / `--input` JSON). Passwords, seal passwords, MFA/OTP/verification/recovery codes and any token/secret/privateKey/authorization/credential-named field (also nested inside `key:=JSON`) are refused in
argv (`SECRET_VIA_ARGV`) — use private JSON input. Success prints
`{"ok":true,"result":...}`; failure prints `{"ok":false,"error":{"code":...}}`
on stderr. `watch` emits JSONL. Outgoing assistant text is prefixed
`[Assistant: NAME]` by the helper.

`post-file channelId=C text=T assistant=NAME path=FILE` runs messages-send then
attachments-upload onto the new topic and prints
`{"ok":true,"result":{"topicId":...,"upload":...}}`. Nothing is retried; if the
upload fails after the send, `UPLOAD_FAILED` carries the `topicId` and the helper's error as `cause`; a send reply without an id reports `SEND_ID_MISSING` (partial, uncertain). Malformed arguments report `USAGE`; `help` works without the helper.

Wrapper tests (fake helper): `python3 -m unittest examples/klatch/test_cli_wrapper.py`.
Run builds and tests on the shared test host only, via `bashy outpost ssh exec`.

Klatch is based on Databag by Roland Osborne
(github.com/balzack/databag), Apache-2.0.
