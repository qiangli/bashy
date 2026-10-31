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

| `post GROUP TEXT\|-` | post TEXT (or stdin with `-`) to the group as the bot |
| `topic GROUP TEXT\|-` | set the group's conversation header to `NAME — TEXT`; empty TEXT restores `NAME` |
| `invite COUNT [REF]` | mint COUNT one-time sign-up links (`<base_url>/?add=TOKEN`) into the vault at REF (default `klatch/signup-links`); prints only the count and REF |

`post` and `topic` take `CONFIG.json GROUP TEXT|-` and print
`{"group": key, "ok": true}`. They act as the config's `"bot"` handle (a normal
entry in `users`, password via vault ref like the others), or as the group
owner if no `bot` is set. Databag only lets the account that hosts a channel set
its subject, so make the bot the group `owner` (e.g. `"owner": "classbot",
"members": [...]`) and provision. These verbs know nothing about who calls them,
so any line-producer composes:

```sh
bashy tutor status 528 --line | bashy klatch topic class.json class -
examples/tutor/chat-status.sh 528 class.json class --every 30   # loop + milestone posts
```

The header is the channel subject, shown as the conversation title on web
(single line, ellipsised when long) and on the phone app. Groups carry their
`key` in the channel data, so a changed header never makes a rerun duplicate
the group.

`setup` and `provision` print JSON: `{"handles": {handle: guid}, "groups":
{key: id}}`. Reruns skip existing accounts, contacts and groups (groups are
matched by subject), so they never duplicate.

Tests (fake HTTP server, no real server needed):
`python3 -m unittest examples/klatch/test_provision.py`

Klatch is based on Databag by Roland Osborne
(github.com/balzack/databag), Apache-2.0.
