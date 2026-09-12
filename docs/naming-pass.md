# The naming pass — what got renamed, and what deliberately did not

Status: **historical naming decision, revised 2026-08-12.** Companion to `command-atlas.md` (the catalog) and
`plan-bashy-coherence-pass.md` (the wider pass).

## The rule

> **Rename only where the name LIES.** A namespaced subcommand is not a collision.

bashy's verb surface grew piecemeal, and an audit found the same word doing several jobs: `run` seven
ways, `verify` four, `promote` two, `supervise` two. It is tempting to "fix" all of it. That would be
churn for its own sake — most of those are *namespaced* (`skills run`, `foreman run`, `schedule run`)
and are unambiguous at the command line. Reusing a verb inside a namespace is how CLIs are supposed to
work.

What is *not* fine is a **top-level verb whose name misdescribes what it does**, because that is what
an agent reads first and reasons from.

## Renamed (2)

### `chat` → **`invoke`**

`chat` does not chat. Its own synopsis always read:

> *"invoke an agent with a single unattended instruction"*

No conversation, no back-and-forth, no session. It is the primitive that **unifies the heterogeneous
agent CLIs** — resolve the tool (claude/codex/opencode/aider/…), inject identity, force bashy as its
shell — and every orchestrator is built on it: `sdlc`, `foreman` and `meet` all call it. (Only `weave`
bypasses it; it drives a PTY.)

The name actively misled: an agent reading `bashy chat` reasonably assumes an interactive session —
which is exactly what **`foreman`** is. Two different things, one of them wearing the other's name.

`invoke` says what it does: **one agent, once, on one instruction.**

### `verify` → **`conform`**

`bashy verify` runs **bashy's own fidelity batteries**: GNU Bash 5.3 compat, POSIX conformance,
VSC-PCTS compliance, agentic benchmark.

So it had claimed *the most general word in the vocabulary* for *the narrowest possible thing —
verifying bashy itself*. A project that **adopts** bashy would reach for `bashy verify` to ask "does
**my** code pass?" and get bash's conformance suites instead. That is not a collision; it is a trap.

`conform` says what it does. The general pass/fail question — *does this project pass?* — is
**`bashy gate`**.

### 2026-08-12 correction: `chat` became a real chat

The July decision described the implementation that existed then. It stopped
being true when `chat` gained a governed interactive session and the
`sessions`, `steer`, `attach`, `interrupt`, and `timeline` controls. At that
point the command really did chat, but the catalog still hid it as if it were
the old one-shot alias.

`chat` is therefore canonical and visible again. `invoke` remains a hidden
compatibility spelling for existing one-shot callers; both still reach the
same governed launcher, and `--instruction` selects the one-shot path.

### Old names still work

`invoke` and `verify` remain as **hidden aliases** (`alias_of` in the atlas, the same machinery as
`docker` → `podman`). Nothing breaks; existing scripts, muscle memory and skill docs keep working. They
are hidden from `bashy commands` and visible with `--all`.

## Deliberately NOT renamed

| word | uses | why it stays |
|---|---|---|
| `run` | `bashy run` (result envelope) · `skills run` · `foreman run` · `schedule run` · `sdlc service run` | Only ONE is top-level. The rest are namespaced and unambiguous. Renaming them would churn every caller to fix a problem nobody has. |
| `promote` | `sdlc promote` (deploy baton) · `skills promote` | Both namespaced. Different nouns, no ambiguity at the CLI. |
| `review` / `status` / `list` / `board` | across weave, sprint, sdlc, meet | Namespaced. **But their JSON schemas differ for the same word** — that is a real problem, and it is a *schema* problem, not a *naming* one. Tracked separately; renaming would not fix it. |
| `supervise` | `bashy supervise` (public) · `sdlc supervise` (hidden internal babysitter) | The public one is being **deleted** — folded into `weave --in-place` (its only honest differentiator is "live tree, not a clone", which is a flag, not a package). The collision resolves itself. |

## 2026-09-12: nouns are singular

> **Nouns are singular.** A front-door verb or subcommand that names a *kind of thing* is
> spelled in the singular (`agent`, `model`, `tool`, `skill`, `secret`, `app`, `person`).
> Enumeration is the `list` subcommand, never an `-s` suffix. A plural spelling is accepted
> as a **hidden alias** of the singular — kept for existing callers, listed only under
> `commands --all` — never canonical, never taught.

Why this clears the bar above. English number is not a shell concept, and the surface had
no rule an agent could apply: `todo` and `sprint` were enumerable lists and singular,
`capability` a matrix and singular, `secrets` a vault and plural, `inbox` a queue and
singular. So every noun's number was a fact to memorise, and a misremembered one was a bare
`command not found` with no suggestion path. Worse, `agent` and `agents` were **two
unrelated verbs** — one letter apart, one an identity helper, the other the fleet
registry — which is a top-level name that lies. Singular was already the spelling of 53 of
the 61 native verbs, and the convention of the CLIs agents already know (`git remote`,
`docker container`, `gh pr`). bash's own plurals (`jobs`, `dirs`, `times`) are all
subcommand-less listers — plural means *print the set* — so a plural verb with
`add`/`rm`/`set` under it did not even follow bash's grammar.

What changed: `agent model tool person skill secret app` are the verbs; `agents models tools
people skills secrets apps` dispatch byte-identically as hidden aliases, as do `messages`
(→ `mb`) and `issue` (→ `todo`, which the work-tracking model already made one record at two
scopes). `agent whoami` moved under the noun. Nothing that worked stopped working.

Exceptions are enumerated, never implied (`coreutils/pkg/atlas/naming_test.go` ratchets
them): any name on the POSIX / bash-builtin / GNU-coreutils surface is frozen by the
standard (`jobs`, `dirs`, `times`, `getopts`, `strings`, `tabs`, `users`, `groups`,
`command`, `who`, `whoami`, …), and `commands` stays plural because `command` **is** one of
them and it is a subcommand-less lister of the `jobs` shape. Storage and wire identifiers —
store directories, `BASHY_*_DIR`, REST paths, scopes, entity kinds, schema strings — are not
command spellings and are untouched. The same rule applies one level down (`weave comment`
vs `weave comments`, `craft fold` vs `craft folds`, `bus subscribe` vs `bus subscriptions`):
those pairs converge on `<noun> list` as each package is next opened.

## Why this is worth writing down

The temptation in a naming pass is to make the table *look* tidy. That instinct is how `fanout` shipped
(a verb with no job, filed under a plausible-looking classification) and how `verify` came to mean four
things. The discipline is the same one the Command Atlas now enforces at init:

> **Which stage do you serve, and what do you do, that nothing else already does?**

A rename is justified when the answer contradicts the name. It is not justified because the same word
appears twice in a table.
