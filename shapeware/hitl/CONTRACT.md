# HITL Contract (`CONTRACT.md`)

Behavioural specification for the `hitl` surface: how an agent puts a typed "needs you" request to its human, how a delegate the human designated answers inside its scope on the human's behalf, and how the human answers, overrides and revokes. The surface is `bashy approve`, the one queue of decisions waiting for a human; the pinned bashy reference is the commit in [SKILL.md](SKILL.md). A-01 to A-06 lift the approval behaviour that existed before this surface was extended; A-07 onward cover the extension. What a command line cannot show is pinned by cited go tests (A-33 to A-39) and listed in the known issues of SKILL.md.

## How assertions are judged

- Run the assertions in order, in one disposable work directory outside any repository, with one fresh HOME and TMPDIR per run. Later assertions read what earlier ones wrote: the approval raised in A-02, the requests raised in A-07, A-15, A-16 and A-30, and the delegation set in A-14.
- Every command is `session KIND approve ARGS`. `session` runs the built `bashy approve ARGS` as the caller `KIND` names and sets only this:

  | KIND | What it sets |
  |---|---|
  | `human` | `BASHY_PRINCIPAL=tester`, no agent marker: the human the requests are for |
  | `stranger` | `BASHY_PRINCIPAL=mallory`, no agent marker: another person on the host |
  | `agent:NAME` | `BASHY_PRINCIPAL=tester BASHY_AGENT=NAME`: an agent session running for tester |

  Nothing else in the environment marks an agent. The agents are `mgr-741` (a sprint manager raising requests), `steward-agent` (the delegate tester designates in A-14) and `intruder` (an agent nobody designated).
- `session gotest PKG NAME` runs one pinned go test and prints exactly `PASS NAME` or `FAIL NAME` (A-33 to A-39). Each `PKG` is a package of the yoke module the implementation pins.
- Output is compared as plain text, standard output then standard error, except where the assertion asks for `--json`: JSON output is compared structurally, and an object holding `"...": "..."` is open (other keys may be present). `...` in text matches any text. `{ID}` binds on its first match (A-02) and is the same value afterwards.

## Output rules

- **Envelope.** Every JSON document `approve` prints carries `"schema_version": "bashy-approve-v1"`. A request view always has `id`, `state`, `token`, `source`, `requester`, `created` and `expires`; a needs request adds `need`, `need_state` and, when offered, `options`; a decided one adds `decision`. Request views are open objects; only the keys an assertion names are judged. The two list envelopes (A-09, A-20) are closed objects: exactly `schema_version` and `approvals`, with the `approvals` array compared element by element, so the number of requests listed is judged.
- **Request ids.** A request is named by its id (32 hexadecimal digits), by a unique prefix of it, or, for a needs request, by `key:K`, where `K` is the `--key` it was raised with. Answer lines print the first eight digits of the id.
- **Answer line.** Deciding a needs request prints `VERDICT ID8`, then ` option OPTION` when an option was chosen, then ` (by AGENT on behalf of HUMAN)` for a delegate's answer, or ` (overrides VERDICT by WHO via VIA)` for an override (A-19, A-22, A-25). `wait` prints the same line (A-27). An approval raised with an argv prints its resume flag `--confirm=TOKEN:yes|no` instead (A-04, A-06).
- **Errors.** A refusal or an invalid request prints one line on standard error and exits 1. The refusal texts are fixed: `ask: this approval has already been decided` (a second answer), `approve: only the human this request is for, or their delegate inside its scope, may answer it` (a caller who may not answer; an agent's refusal adds ` (AGENT is not HUMAN's delegate for this request)`), `approve: only the human may designate or revoke a delegate`, and `approve: only the human may override a decision`.
- **Human-read text.** `list` and `show` without `--json` are for people; this contract judges `show` where its lines are fixed and `list` only through `--json`.

## Definitions (input)

- A **needs request** is raised with `approve request --kind KIND --title TITLE` and no argv. `KIND` is exactly one of `decision`, `approval`, `action`, `info`; any other word is refused (A-11). It may carry options `--option ID[:LABEL]` (repeatable), `--recommend ID` (must be one of the options), refs `--ref REF` (repeatable), `--deadline` (an RFC 3339 time or a duration from now), `--urgency` (exactly one of `low`, `normal`, `urgent`; default `normal`), risk tags `--risk TAG` (repeatable), `--sprint N` and `--key K`. The input is the command line alone; A-07 raises a request with every field.
- The **human** of a request is the principal of the process that raised it (here `tester`). The **requester** is the agent that raised it (`need.from`), when an agent did.
- The **non-delegable set** is exactly {`deploy`, `security`, `credentials`, `money`, `delete`, `access`}. Tags are compared after trimming and lower-casing, and are stored that way. A tag is non-delegable when it is in this set and the delegation does not list it under `--allow-risk`; any other tag is delegable. Boundary pair: `Security` (stored `security`) keeps A-16 with the human, while a request with no risk tag in the delegation's kinds and sprint goes to the delegate (A-15).
- A **delegation** is one record per human: `approve delegate --to NAME [--kind K]... [--sprint N]... [--allow-risk TAG]... [--ttl D]`. Empty kinds or sprints mean all. It is **active** until revoked or past its ttl.
- A request is **in scope** of a delegation when its kind is in the delegation's kinds, its sprint is in the delegation's sprints, and none of its risk tags is non-delegable.
- **Routes and states.** At raise time a request in scope of the active delegation is routed `delegate` (state `delegated`); any other is routed `human` (state `open`). A delegated request whose deadline passed, or whose delegation was revoked, expired, given to another agent or narrowed so the request is out of scope, is `escalated`; an open one past its deadline is `overdue`. A decided request is `answered-by-delegate` or `answered-by-human`.

## Service rules

- **One queue.** A needs request is an approval: same store, same ids, same first-decider-wins decision, same room timeline events. Raising again with the same key while the first is undecided returns the first (A-08, A-09).
- **Who may answer.** The human may answer any of their requests. On the board the human is a signed-in owner: not the unauthenticated loopback default, not a paired device, not a cloud caller without a signed owner relationship. An agent may answer only as the active delegate, only a request routed to it and still in its scope (A-17, A-18, A-19, A-31). Another person may not (A-23). The board answers as the signed-in host owner, never as the unauthenticated loopback default (A-36, A-37, A-38).
- **On behalf.** A delegate's answer is recorded as made by the delegate on the human's behalf (`decision.by`, `decision.on_behalf_of`, `decision.via` `delegate`) and listed by `approve list --all --on-behalf` (A-19, A-20).
- **Human only.** Only the human designates, revokes (A-13) or overrides (A-21). An override replaces the standing decision and keeps the one it replaced in `prior_decisions` (A-22, A-28). Revoking takes effect at once: what was delegated escalates to the human and the former delegate is refused (A-31, A-33).
- **The human is alerted.** Every undecided request in state `open`, `overdue` or `escalated` is listed on the sprint board web UI as Needs you, with its options and recommendation (A-36) and, for its sprint, on `bashy sprint show` (A-39). A delegated request is not.
- **The requester resumes.** `approve wait` returns the decision as soon as one stands (A-27), and the requester agent is notified of the answer (A-35).
- **Approvals with an argv** behave as before this surface: resume flags, no option, no on-behalf check (A-01 to A-06).

### A-01
An empty queue says so.
```console
$ session human approve list
nothing pending
```
exit: 0

### A-02
An approval for an argv is raised and its id printed.
```console
$ session agent:mgr-741 approve request -- rm -rf build
{ID}
```
exit: 0

### A-03
`show` names the argv and that it is pending.
```console
$ session human approve show {ID}
id:        {ID}
state:     pending
source:    confirm
token:     ...
argv:      rm -rf build
requester: ...
prompt:    approval requested for rm -rf build
```
exit: 0

### A-04
Allowing an argv approval prints its resume flag, then what was decided.
```console
$ session human approve allow {ID}
--confirm=...:yes
allow ... (token ...)
```
exit: 0

### A-05
A second answer is refused: the first decider wins.
```console
$ session human approve allow {ID}
ask: this approval has already been decided
```
exit: 1

### A-06
`wait` on a decided approval returns its resume flag at once.
```console
$ session agent:mgr-741 approve wait {ID}
--confirm=...:yes
```
exit: 0

### A-07
A sprint manager raises a typed needs request with every field: the hotfix decision, non-delegable because of its `deploy` tag.
```console
$ session agent:mgr-741 approve request --kind decision --title "Ship the check-in hotfix?" --option "ship:Ship now" --option hold --recommend ship --ref sprint:741 --ref story:2e4900ff3135 --deadline 4h --urgency urgent --risk deploy --sprint 741 --key hotfix --json
{"schema_version":"bashy-approve-v1","state":"pending","source":"needs","sprint":"741","need":{"kind":"decision","title":"Ship the check-in hotfix?","recommend":"ship","refs":["sprint:741","story:2e4900ff3135"],"urgency":"urgent","risk":["deploy"],"key":"hotfix","human":"tester","from":"mgr-741","route":"human","...":"..."},"options":[{"id":"ship","name":"Ship now"},{"id":"hold"}],"need_state":"open","...":"..."}
```
exit: 0

### A-08
Raising the same key again returns the open request instead of a second one.
```console
$ session agent:mgr-741 approve request --kind decision --title "Ship the check-in hotfix?" --key hotfix
...
```
exit: 0

### A-09
The queue holds one pending request: the hotfix decision.
```console
$ session human approve list --json
{"schema_version":"bashy-approve-v1","approvals":[{"need":{"key":"hotfix","...":"..."},"need_state":"open","...":"..."}]}
```
exit: 0

### A-10
`show` gives the human the question, the options with the recommendation, the refs and the risk.
```console
$ session human approve show key:hotfix
id:        ...
state:     pending
source:    needs
token:     ...
argv:      needs decision Ship the check-in hotfix?
sprint:    741
kind:      decision
title:     Ship the check-in hotfix?
need:      open (route human)
option:    ship Ship now (recommended)
option:    hold
ref:       sprint:741
ref:       story:2e4900ff3135
risk:      deploy
deadline:  ...
human:     tester
requester: ...
prompt:    Ship the check-in hotfix?
```
exit: 0

### A-11
A kind outside the four is refused.
```console
$ session agent:mgr-741 approve request --kind vote --title x
approve: --kind must be one of decision, approval, action, info, got "vote"
```
exit: 1

### A-12
With no delegation every request comes to the human.
```console
$ session human approve delegate
no delegate: every needs request comes to you
```
exit: 0

### A-13
An agent cannot designate a delegate, itself included.
```console
$ session agent:steward-agent approve delegate --to steward-agent
approve: only the human may designate or revoke a delegate
```
exit: 1

### A-14
The human designates `steward-agent` for decisions and information on sprint 741.
```console
$ session human approve delegate --to steward-agent --kind decision,info --sprint 741
active to=steward-agent kinds=decision,info sprints=741 allow-risk=none
```
exit: 0

### A-15
A request in the delegation's scope is routed to the delegate.
```console
$ session agent:mgr-741 approve request --kind decision --title "Review the lifted specs" --option accept --option rework --sprint 741 --key review --json
{"schema_version":"bashy-approve-v1","need":{"route":"delegate","delegate":"steward-agent","human":"tester","...":"..."},"need_state":"delegated","...":"..."}
```
exit: 0

### A-16
A request with a non-delegable tag, in any case, stays with the human even inside the delegation's sprint.
```console
$ session agent:mgr-741 approve request --kind approval --title "Push the security hotfix" --risk Security --sprint 741 --key sec --json
{"schema_version":"bashy-approve-v1","need":{"route":"human","risk":["security"],"...":"..."},"need_state":"open","...":"..."}
```
exit: 0

### A-17
An agent nobody designated cannot answer.
```console
$ session agent:intruder approve allow key:review --option accept
approve: only the human this request is for, or their delegate inside its scope, may answer it (intruder is not tester's delegate for this request)
```
exit: 1

### A-18
The delegate cannot answer a request outside its scope.
```console
$ session agent:steward-agent approve allow key:sec
approve: only the human this request is for, or their delegate inside its scope, may answer it (steward-agent is not tester's delegate for this request)
```
exit: 1

### A-19
The delegate answers an in-scope request on the human's behalf.
```console
$ session agent:steward-agent approve allow key:review --option accept --reason "examples pass"
allow ... option accept (by steward-agent on behalf of tester)
```
exit: 0

### A-20
The human lists what was decided on their behalf.
```console
$ session human approve list --all --on-behalf --json
{"schema_version":"bashy-approve-v1","approvals":[{"need":{"key":"review","...":"..."},"decision":{"verdict":"allow","by":"steward-agent","via":"delegate","option_id":"accept","reason":"examples pass","on_behalf_of":"tester","...":"..."},"need_state":"answered-by-delegate","...":"..."}]}
```
exit: 0

### A-21
The delegate cannot override, not even its own decision.
```console
$ session agent:steward-agent approve deny key:review --override
approve: only the human may override a decision
```
exit: 1

### A-22
The human overrides the delegate's decision.
```console
$ session human approve allow key:review --option rework --override --reason "needs examples"
allow ... option rework (overrides allow by steward-agent via delegate)
```
exit: 0

### A-23
Another person cannot answer the human's request.
```console
$ session stranger approve allow key:sec
approve: only the human this request is for, or their delegate inside its scope, may answer it
```
exit: 1

### A-24
The human answers the non-delegable approval.
```console
$ session human approve allow key:sec --reason reviewed
allow ...
```
exit: 0

### A-25
An option the request does not offer is refused, and the request stays open.
```console
$ session human approve allow key:hotfix --option nope
approve: option "nope" is not one of this request's options
```
exit: 1

### A-26
The human chooses the recommended option.
```console
$ session human approve allow key:hotfix --option ship
allow ... option ship
```
exit: 0

### A-27
The waiting manager resumes with the standing decision.
```console
$ session agent:mgr-741 approve wait key:review
allow ... option rework (overrides allow by steward-agent via delegate)
```
exit: 0

### A-28
The record keeps the delegate's decision that the human overrode.
```console
$ session human approve show key:review --json
{"schema_version":"bashy-approve-v1","decision":{"verdict":"allow","by":"tester","via":"human","option_id":"rework","overrides":"allow by steward-agent via delegate","...":"..."},"prior_decisions":[{"verdict":"allow","by":"steward-agent","via":"delegate","option_id":"accept","on_behalf_of":"tester","...":"..."}],"need_state":"answered-by-human","...":"..."}
```
exit: 0

### A-29
The human revokes the delegation.
```console
$ session human approve delegate --revoke
revoked to=steward-agent kinds=decision,info sprints=741 allow-risk=none
```
exit: 0

### A-30
After the revoke a request that was in scope goes to the human.
```console
$ session agent:mgr-741 approve request --kind info --title "Revoked: nothing is delegated" --sprint 741 --key fyi --json
{"schema_version":"bashy-approve-v1","need":{"route":"human","...":"..."},"need_state":"open","...":"..."}
```
exit: 0

### A-31
The former delegate is refused.
```console
$ session agent:steward-agent approve allow key:fyi
approve: only the human this request is for, or their delegate inside its scope, may answer it (steward-agent is not tester's delegate for this request)
```
exit: 1

### A-32
The human acknowledges the information request.
```console
$ session human approve allow key:fyi
allow ...
```
exit: 0

### A-33
A request delegated before a revoke escalates to the human at once.
```console
$ session gotest github.com/qiangli/yoke/pkg/ask TestNeedRevokeEscalatesAtOnce
PASS TestNeedRevokeEscalatesAtOnce
```
exit: 0

### A-34
A delegated request past its deadline escalates to the human.
```console
$ session gotest github.com/qiangli/yoke/pkg/ask TestNeedDeadlineEscalatesDelegated
PASS TestNeedDeadlineEscalatesDelegated
```
exit: 0

### A-35
The requester agent is notified of the answer.
```console
$ session gotest github.com/qiangli/yoke/pkg/ask TestNeedAnswerWakesRequester
PASS TestNeedAnswerWakesRequester
```
exit: 0

### A-36
The sprint board lists the requests waiting on the human and refuses an answer from the unauthenticated loopback default.
```console
$ session gotest github.com/qiangli/yoke/pkg/webconsole TestBoardOverviewCarriesNeedsYou
PASS TestBoardOverviewCarriesNeedsYou
```
exit: 0

### A-37
The signed-in owner answers on the board; a second answer conflicts and the request leaves the alert.
```console
$ session gotest github.com/qiangli/yoke/pkg/webconsole TestBoardAnswerSignedInOwner
PASS TestBoardAnswerSignedInOwner
```
exit: 0

### A-38
A cloud caller without a signed owner relationship cannot answer on the board.
```console
$ session gotest github.com/qiangli/yoke/pkg/webconsole TestBoardAnswerRefusesUnsignedCloudCaller
PASS TestBoardAnswerRefusesUnsignedCloudCaller
```
exit: 0

### A-39
`bashy sprint show` lists the sprint's open requests.
```console
$ session gotest github.com/qiangli/yoke/pkg/weave TestRenderSprintNeedsListsOpenRequests
PASS TestRenderSprintNeedsListsOpenRequests
```
exit: 0
