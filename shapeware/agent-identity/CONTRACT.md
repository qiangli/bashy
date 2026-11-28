# Agent identity Contract (`CONTRACT.md`)

Behavioural specification for the `agent-identity` surface: who a bashy front-door verb runs as when an agent calls it. The pinned bashy reference is commit `a5b5a943502db5795e3c699b23ad64d185f76a63`. Assertions cover a `bashy <tool>` launch's stated session form, the naming of an agent session that declared nothing, the verbs run under that name, and the refusal of a session that cannot be named. What a command line cannot show is listed in the known issues of [SKILL.md](SKILL.md).

## How assertions are judged

- Run the assertions in order, in one disposable work directory outside any repository, with one fresh HOME and TMPDIR per run. Later assertions read what earlier ones wrote: the sessions named in A-02, A-05 and A-07, the claim taken in A-04 and the message sent in A-09.
- Every command is `session KIND VERB ARGS`. `session` runs the built `bashy VERB ARGS` as the caller `KIND` names, and sets only what that caller's own CLI would set:

  | KIND | What it sets |
  |---|---|
  | `human` | no agent marker |
  | `claude:ID` | `CLAUDECODE=1 CLAUDE_CODE_SESSION_ID=ID` |
  | `codex:ID` | `CODEX_THREAD_ID=ID` |
  | `unnamed` | `GEMINI_CLI=1` |
  | `unnamed+name` | `GEMINI_CLI=1 BASHY_AGENT=my-own-name` |
  | `unnamed+off` | `GEMINI_CLI=1 BASHY_ADHOC_IDENTITY=off` |

  Nothing else in the environment marks an agent, and bashy's hints are off: the judge discards hint lines in any case, so what a hint says is asserted through a cited test (A-10).
- `session gotest PKG NAME` runs one pinned go test of the implementation and prints exactly `PASS NAME` or `FAIL NAME` (A-10, A-23, A-24). `PKG` is a package of the implementation or of a module its `go.mod` requires; the tests cited here are in the implementation (A-10) and in the yoke module it pins (A-23, A-24).
- Output is compared as plain text: everything the command wrote to standard output, then everything it wrote to standard error. So a line bashy prints on standard error before the verb runs (the naming line, a hint) appears after the verb's own output in an expected block. `...` matches any text. `{TIME}` matches a timestamp. An assertion with an empty block is judged by its exit code alone.
- The names in the expected output are literal because they are defined values, not samples: see "Definitions".

## Output rules

- **Envelope.** This surface asserts plain text only. No assertion compares a JSON object, so there is no open or closed object rule to state; the one JSON line in A-10 is compared as text.
- **Naming line.** The first front-door verb of a session that is given a name prints one line on standard error, before the verb runs: `bashy: this TOOL session is NAME`, followed by how others reach it and how to read mail. It is printed once per session, not once per command (A-02, A-03).
- **Refusals.** A refusal is plain text on standard error with exit 2. Its first line starts with `bashy VERB: unattributed agent session: an agent id is required` and names the harness; the lines after it are the naming guidance, the same text for every verb (A-14, A-15).

## Definitions (input)

- A **caller** is one of the kinds in the table above. An **agent session** is a caller whose environment carries an agent marker. A caller with no marker is not an agent and this surface changes nothing for it (A-01).
- A **declared identity** is a name the caller set for itself before calling. This surface treats `BASHY_AGENT` as the declaration; a declared name is used as given, whether or not it is registered anywhere (A-18).
- A **session key** is what stays the same across every command of one agent session. Here it is the session id the agent's own CLI exposes: `CLAUDE_CODE_SESSION_ID` for claude, `CODEX_THREAD_ID` for codex. An agent session with no declared identity and no session key is **unidentifiable**: the `unnamed` kind (A-14).
- The **name** of an agent session with a session key and no declared identity is `TOOL-HEX`. `HEX` is the first eight hexadecimal digits of the session's instance UUID, and that UUID is the version 5 UUID of the text `TOOL`, a NUL byte, `session:` and the session id, in the namespace `6a1f0c1e-3c0b-5d53-9a4e-7b1d2f4c8e90`. So the name is a function of the tool and the session id alone: the same session gets the same name on every command (A-02, A-04), another session id gets another name (A-05), and another tool gets another name (A-07). The values this contract uses: claude `session-one` is `claude-8e84f3ea`, claude `session-two` is `claude-b272c2c6`, codex `thread-9` is `codex-0fe06ab4`.
- **Coordination verbs** are exactly the set {`mb`, `messages`, `inbox`, `notify`, `meet`, `claim`, `sprint`, `todo`, `issue`, `weave`, `handoff`, `resume`, `delegate`, `coach`, `foreman`, `supervise`, `ask`, `bus`, `feed`, `herald`, `activity`}: a verb is a coordination verb when its name equals one of these, and is not otherwise. Boundary pair: `claim` is in the set and is refused for an unidentifiable session (A-14), while `instance` is not in the set and runs for the same session (A-16).
- A **request for help** is a verb followed by `--help`, `-h` or `help` before any `--`. It is never refused (A-17).
- A **message body** is plain text; [examples/message.txt](examples/message.txt) is the one this contract sends (A-09) and reads back (A-12).
- The **off switch** is `BASHY_ADHOC_IDENTITY=off`. With it, no session is named and no verb is refused by this surface (A-19).

## Service rules

- **Naming happens before the verb.** An agent session with a session key and no declared identity is named on its first front-door verb, whatever the verb. The verb then runs as that name, and so does everything it starts.
- **The name is who acted.** A claim taken by a named session is held under its name (A-04, A-05). A message it sends is signed with it and one sent to its name reaches it (A-09, A-12).
- **One instance per session.** Each named session has one instance whose label is its name; a later command of the same session adds none (A-06, A-08).
- **Mail is announced once.** With hints on, the first verb after mail arrives prints one hint line saying how many are unread and from whom, never a message body; the next verb does not repeat it (A-10). `inbox` shows the message and does not consume it (A-12).
- **Unidentifiable sessions.** An unidentifiable agent session is refused on a coordination verb and nothing is recorded for it. On any other verb, on a request for help, and when it runs the shell itself (`-c`), it behaves as it did before this surface existed (A-16, A-17, A-22).
- **A `bashy TOOL` launch** opens that tool's own terminal session under an id of the same form, `TOOL-HEX`, new on every launch. This contract asserts the form the tool's resolved launch and help state (A-20, A-21) and cites the test that pins how a live session takes its id (A-23); the line a live terminal session prints is KI-1.
- **A tool with no session key.** A session of a tool whose CLI exposes no session id is keyed on the tool's own process above the command, its pid together with its start time. A command line cannot place such a process, so this contract cites the test that pins it (A-24, KI-2).

### A-01
A caller that is not an agent is not named and not refused.
```console
$ session human claim list
claim: nobody is working on this host
```
exit: 0

### A-02
The first verb of an agent session that declared nothing names it, then runs.
```console
$ session claude:session-one claim list
claim: nobody is working on this host
bashy: this claude session is claude-8e84f3ea — others reach it with `bashy mb send claude-8e84f3ea "..."`; read with `bashy inbox`
```
exit: 0

### A-03
The same session is not told its name again.
```console
$ session claude:session-one claim list
claim: nobody is working on this host
```
exit: 0

### A-04
A claim taken with no declared name is held under the session's name.
```console
$ session claude:session-one claim host:demo --intent probe
claim: host:demo held on ... by claude-8e84f3ea (lease, epoch 1)
```
exit: 0

### A-05
Another session of the same tool gets a different name, and sees the first one's claim under its name.
```console
$ session claude:session-two claim list
HOLDER           KIND  NAME       STATE  MODE   EPOCH  INTENT
claude-8e84f3ea  host  host:demo  live   lease  1      probe
bashy: this claude session is claude-b272c2c6 — others reach it with `bashy mb send claude-b272c2c6 "..."`; read with `bashy inbox`
```
exit: 0

### A-06
Each session has one instance, labelled with the first group of its UUID.
```console
$ session claude:session-one instance list
UUID                                  LABEL            FAMILY  STATE
8e84f3ea-f106-50b6-8c19-2c7e1cf60c51  claude-8e84f3ea  claude  active
b272c2c6-9097-50b5-8d49-0fea7b21d7bd  claude-b272c2c6  claude  active
```
exit: 0

### A-07
A session of another tool is named after that tool.
```console
$ session codex:thread-9 claim list
HOLDER           KIND  NAME       STATE  MODE   EPOCH  INTENT
claude-8e84f3ea  host  host:demo  live   lease  1      probe
bashy: this codex session is codex-0fe06ab4 — others reach it with `bashy mb send codex-0fe06ab4 "..."`; read with `bashy inbox`
```
exit: 0

### A-08
A second command of that session adds no instance.
```console
$ session codex:thread-9 instance list
UUID                                  LABEL            FAMILY  STATE
8e84f3ea-f106-50b6-8c19-2c7e1cf60c51  claude-8e84f3ea  claude  active
b272c2c6-9097-50b5-8d49-0fea7b21d7bd  claude-b272c2c6  claude  active
0fe06ab4-...  codex-0fe06ab4   codex   active
```
exit: 0

### A-09
A named session sends to another by its name, and is told the delivery is not yet confirmed.
```console
$ session claude:session-two mb send claude-8e84f3ea "$(cat examples/message.txt)"
  posted, but delivery UNVERIFIED — no read cursor yet for: claude-8e84f3ea
  warning: no read evidence for claude-8e84f3ea — the board accepted this; that is not proof it was delivered
```
exit: 0

### A-10
Mail sent to a named session is announced on its next verb, once, without the body (cited test; the judge discards hint lines).
```console
$ session gotest ./internal/agentos TestUnreadHintReachesANativeAgentSession
PASS TestUnreadHintReachesANativeAgentSession
```
exit: 0

### A-12
The recipient reads the message, signed with the sender's name.
```console
$ session claude:session-one inbox
{TIME} [mb:1] instance/b272c2c6-9097-50b5-8d49-0fea7b21d7bd (label: claude-b272c2c6; uuid: b272c2c6-9097-50b5-8d49-0fea7b21d7bd; family: claude; bindings: claude) → instance/8e84f3ea-f106-50b6-8c19-2c7e1cf60c51 (mb)
ping from two
```
exit: 0

### A-13
The sender has no mail of its own.
```console
$ session claude:session-two inbox
nothing new in any channel for instance/b272c2c6-9097-50b5-8d49-0fea7b21d7bd
```
exit: 0

### A-14
An unidentifiable agent session is refused on a coordination verb, with how to name itself.
```console
$ session unnamed claim list
bashy claim: unattributed agent session: an agent id is required: running under gemini with no declared identity, no session id and no gemini process above this command
name this session, then run the command again:
  BASHY_AGENT=NAME bashy <command> ...     a name of your choosing, on every call
  bashy agent add NAME --tool T --model M  register it once, to be addressable by that name
  BASHY_ADHOC_IDENTITY=off                 run as before this check (the bare tool name)
```
exit: 2

### A-15
The refusal is the same for another coordination verb, and nothing is posted.
```console
$ session unnamed mb post x
bashy mb: unattributed agent session: an agent id is required: running under gemini with no declared identity, no session id and no gemini process above this command
name this session, then run the command again:
  BASHY_AGENT=NAME bashy <command> ...     a name of your choosing, on every call
  bashy agent add NAME --tool T --model M  register it once, to be addressable by that name
  BASHY_ADHOC_IDENTITY=off                 run as before this check (the bare tool name)
```
exit: 2

### A-16
The same session runs a verb that is not a coordination verb, and no instance was opened for it.
```console
$ session unnamed instance list
UUID                                  LABEL            FAMILY  STATE
8e84f3ea-f106-50b6-8c19-2c7e1cf60c51  claude-8e84f3ea  claude  active
b272c2c6-9097-50b5-8d49-0fea7b21d7bd  claude-b272c2c6  claude  active
0fe06ab4-...  codex-0fe06ab4   codex   active
```
exit: 0

### A-17
A request for a coordination verb's help is not refused.
```console
$ session unnamed claim --help
---
name: bashy-claim
...
```
exit: 0

### A-18
An agent session that chose a name for itself is left alone.
```console
$ session unnamed+name claim list
HOLDER           KIND  NAME       STATE  MODE   EPOCH  INTENT
claude-8e84f3ea  host  host:demo  live   lease  1      probe
```
exit: 0

### A-19
With the off switch, the same session is not refused.
```console
$ session unnamed+off claim list
HOLDER           KIND  NAME       STATE  MODE   EPOCH  INTENT
claude-8e84f3ea  host  host:demo  live   lease  1      probe
```
exit: 0

### A-20
The resolved launch of `bashy claude` states the per-launch session form.
```console
$ session claude:session-one claude --dry-run
tool: claude
model: (vendor default)
binding: claude
session: claude-<8 hex>, new per launch
argv: ...
Steer it: bashy chat steer claude "..."   (watch: bashy chat attach claude)
```
exit: 0

### A-21
The tool's help states it too, with how to reach one session among several.
```console
$ session human claude --help
Usage:
  bashy claude [--model MODEL] [--yolo|--no-yolo] [PROMPT...]
...
bypass on. Each launch gets its own session id, claude-<8 hex> (shown in the
launch banner and by bashy chat sessions). While one claude session is live,
the tool name reaches it: bashy chat steer claude "...".
...
  session:  claude-<8 hex>, new per launch
...
With several live, use the full id: bashy chat sessions
```
exit: 0

### A-22
The shell itself is not part of this surface: an unidentifiable session runs it.
```console
$ session unnamed -c 'echo plain shell'
plain shell
```
exit: 0

### A-23
A live `bashy TOOL` session takes its id from the UUID its instance opens with (cited test, yoke `pkg/chat`).
```console
$ session gotest github.com/qiangli/yoke/pkg/chat TestJoinAdHocDerivesTheIDFromTheInstanceUUID
PASS TestJoinAdHocDerivesTheIDFromTheInstanceUUID
```
exit: 0

### A-24
A session of a tool with no session id is named from the harness process above the command (cited test, yoke `pkg/bus`).
```console
$ session gotest github.com/qiangli/yoke/pkg/bus TestAdHocIdentityFromTheHarnessAncestor
PASS TestAdHocIdentityFromTheHarnessAncestor
```
exit: 0
