---
name: feed-setup
description: 'Set up bashy feed for a person end to end: pairing, a publish sink, and one or more messaging channels (Telegram, Discord, Slack, Matrix chat) over OpenClaw, driven by `bashy feed doctor` until it exits 0. Use when asked to set up, finish or repair feed delivery. Idempotent: re-run it any time; it only does what the doctor reports missing.'
metadata:
  requires: "has=bashy"
---

# feed-setup — get `bashy feed doctor` to exit 0

This skill composes existing commands only. It adds nothing: the loop driver
is `bashy feed doctor`, which reports pairing, signing key, sinks, channels
and secrets as `configured` or `missing`, prints the exact `next:` command for
each missing check, and exits non-zero until everything is configured.

## Hard rules

These are rules, not advice:

1. **Never echo, log, paste, print or commit a secret.** Not in a command
   line, not in a file in a repo, not in your reply, not in a commit message.
   A bot token typed into argv lands in shell history and `ps`.
2. **Secrets travel only `bashy ask` → `bashy secret`.** `bashy ask` collects
   the value from the human on a channel you cannot read and returns a PATH,
   never the value. Feed that path to `bashy secret set NAME` on stdin, then
   delete it. Never pass `--stdout` to `bashy ask`; never `cat` the path.
3. **Ask before creating anything public** — a GitHub repo, a bot, a channel
   invite. State what will be created and where, and wait for an explicit yes.
4. **Escalate human-only steps verbatim.** Accounts, logins, @BotFather, app
   and bot creation, OAuth consent and invites are done by the human. Tell them
   exactly what to do, in the words below, and wait; do not improvise around a
   missing account or token.
5. **Matrix chat** is the messaging channel. Always say "Matrix chat" to the
   human — bare "matrix" means the cloudbox matrix tunnel, a different thing.

Run `bashy <cmd> --help` before relying on a flag; do not invent ones.

## 0. The loop

Run the doctor first, and again after every step below. Do only the steps
whose area it reports `missing`, in the order it lists them.

```bsh
bashy feed doctor          # text: one line per check, "next:" under each missing one
bashy feed doctor --json   # envelope bashy-feed-doctor-v1, for parsing
```

Stop when `bashy feed doctor` exits 0 (step 4). Everything below is safe to
re-run: each step checks before it creates.

## 1. Pairing (area `pairing`)

Pairing links this host to the portal so posts are indexed.

```bsh
bashy feed doctor --json   # is "pairing" configured?
```

If it is missing, this is a human-only step. Tell the human, verbatim:

> This machine is not paired yet. In your own terminal, run `bashy login` and
> paste the one-time invite code from the portal's "Generate invite code"
> dialog. Tell me when it says it is paired.

Do not run `bashy login` for them and never ask them to paste the code to you.
Re-run the doctor when they confirm.

## 2. Sink (area `sinks`)

A sink is where signed posts are published. Offer the two kinds and let the
human pick one.

### 2a. GitHub feed repo (via `gh`)

```bsh
gh auth status             # human-only if not logged in: tell them to run `gh auth login`
gh repo view OWNER/REPO --json url,visibility   # reuse an existing repo if they have one
```

Creating a repo is a public action. Say exactly what you will create
(`OWNER/REPO`, public or private) and wait for an explicit yes. Only then:

```bsh
gh repo create OWNER/REPO --private --add-readme --description "bashy feed"
bashy feed sink add github NAME "$(gh repo view OWNER/REPO --json sshUrl --jq .sshUrl)" --branch main
```

### 2b. rclone remote

```bsh
bashy rclone listremotes   # reuse an existing remote if one fits
```

If none fits, `bashy rclone config` is interactive and cloud logins are
human-only: tell the human to run `bashy rclone config` in their own terminal
and name the remote. Then:

```bsh
bashy feed sink add rclone NAME REMOTE:path/to/feed
```

Pass `--public-url BASE` to either `sink add` when the sink's files are served
over http(s). Re-run the doctor; a sink it cannot reach prints its own `next:`
(`git ls-remote …` or `bashy rclone config`).

## 3. Messaging (areas `channels` and `secrets`)

Ask which of **Telegram, Discord, Slack, Matrix chat** the human wants — one or
more. These four are the first-release set; the doctor reports any other
`--channel` as missing (`next: bashy feed channel rm NAME`).

### 3a. OpenClaw (once, for all four)

Delivery runs through the `openclaw:send` tool command
(`examples/fleet/tools/openclaw.yaml`). OpenClaw needs **Node >= 24.16** —
check before installing:

```bsh
node --version || true                 # need v24.16.0 or newer
bashy mise use -g node@24              # only if node is missing or older
npm install -g openclaw@latest         # the doctor's documented install
bashy tool add --force examples/fleet/tools/openclaw.yaml   # only if `bashy tool list` lacks openclaw:send
openclaw gateway start                 # the doctor probes it at 127.0.0.1:18789
```

Ask before installing anything globally if the human has not already said yes.

### 3b. Collect a token (the one pattern every channel uses)

Every bot token goes through this pattern, and nothing else:

```bsh
p=$(bashy ask --name TELEGRAM_BOT_TOKEN --prompt "Telegram bot token from @BotFather") &&
  bashy secret set TELEGRAM_BOT_TOKEN < "$p"
rm -f "$p"
```

`bashy ask` hides the typed value and prints only the path. Substitute the
NAME and prompt per channel below. To hand the vaulted value to the gateway,
restart it in an environment rendered by the vault — the value is never
written to argv or to a file you create:

```bsh
openclaw gateway stop || true
eval "$(bashy secret env)" && openclaw gateway start
```

### Telegram

Tell the human, verbatim:

> In Telegram, open a chat with @BotFather, send `/newbot`, and follow its
> prompts for a name and a username ending in "bot". It replies with a token.
> Do not paste the token to me — a secure prompt will ask for it.

Collect it with 3b as `TELEGRAM_BOT_TOKEN`, restart the gateway, then tell
the human:

> Open your new bot in Telegram and send it `/start`.

Detect the chat id from OpenClaw's pairing queue (no token in any URL):

```bsh
openclaw pairing list telegram          # the /start shows up with its chat id and a code
openclaw pairing approve telegram CODE  # approve only the code the human confirms is theirs
```

Channel args: `--channel telegram --target CHAT_ID`.

### Discord

Tell the human, verbatim:

> At https://discord.com/developers/applications create an application, open
> Bot, and click Reset Token to get a bot token (do not paste it to me). Turn
> on Message Content Intent. Under OAuth2 → URL Generator pick the `bot`
> scope with Send Messages and View Channels, open the URL, and invite the bot
> to your server. Then, with Developer Mode on (Settings → Advanced),
> right-click the target channel and Copy Channel ID.

Creating the invite is the human's action. Collect the token with 3b as
`DISCORD_BOT_TOKEN`. The channel id is not a secret: the human may tell you
it directly. Restart the gateway.

Channel args: `--channel discord --target channel:CHANNEL_ID`.

### Slack

Tell the human, verbatim:

> At https://api.slack.com/apps create an app in your workspace. Under OAuth
> & Permissions add the `chat:write` bot scope and install it to the
> workspace; copy the Bot User OAuth Token (starts `xoxb-`). If OpenClaw asks
> for Socket Mode, also create an app-level token (starts `xapp-`) with
> `connections:write`. Invite the app to the target channel with
> `/invite @APP`. Do not paste tokens to me.

Collect `SLACK_BOT_TOKEN` (and `SLACK_APP_TOKEN` if used) with 3b, and the
channel id the same way as Discord. Restart the gateway.

Channel args: `--channel slack --target channel:CHANNEL_ID`.

### Matrix chat

Tell the human, verbatim:

> For Matrix chat, create (or use) a separate account for the bot on your
> homeserver, sign in as it, and copy its access token (Element: Settings →
> Help & About → Access Token). Invite that account to the room you want
> updates in and note the room id (Room settings → Advanced, starts `!`).
> Do not paste the token to me.

Collect `MATRIX_ACCESS_TOKEN` with 3b; the room id is not a secret and the
human may tell you it directly. Restart the gateway.

Channel args: `--channel matrix --target room:ROOM_ID`.

### 3c. Register the channel and send ONE test message

Confirm the target arguments with `openclaw message send --help` first, then:

```bsh
bashy feed channel add NAME --cmd openclaw:send --args '--channel telegram --target CHAT_ID'
bashy tool cmd run openclaw:send --yes -- --channel telegram --target CHAT_ID \
  --message "bashy feed is connected. Reply here if you got this."
```

Send exactly one test message per channel, then ask the human to confirm they
received it. If they did not, do not resend in a loop: re-run the doctor and
fix what it reports (gateway, secret binding, target). `bashy feed channel
add --help` covers cadence flags (`--interval`, `--max`, `--quiet`,
`--immediate`); keep the defaults unless the human asks.

## 4. Done

```bsh
bashy feed doctor          # must exit 0
```

Setup is complete only when this exits 0 and the human has confirmed each
test message. Report which sink and which channels were configured — by name,
never by token.
