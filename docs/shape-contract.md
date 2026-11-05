# Software Shaping contract (Sprint 564 H1)

One lifecycle serves the web UI, local `bashy shape`, and remote REST calls.
This is the implementation contract for H2–H5 and H7, not a claim that those
features have shipped. The sprint card and stories remain authoritative.

## Shared lifecycle and interfaces

Extract the candidate lifecycle from `webconsole` into a reusable service in
`yoke/pkg/shape`. Keep the existing state, verifier, evidence and `src` stores;
there is no second CLI candidate store. The service receives app metadata,
caller context and the existing delegate/verifier/activator dependencies.
Local CLI calls it directly. HTTP handlers call it after their existing guards.
`--remote <console>` uses authenticated HTTP instead of starting a local console.

| CLI | Existing REST operation or implementation |
| --- | --- |
| `status APP` | `GET /api/shape/{app}` |
| `wish APP REQUEST` | `POST /api/shape/{app}/candidates` |
| `wish APP --from WORKSPACE` | Import a hand-made candidate through the same lifecycle |
| `show APP ID` | `GET …/candidates/{id}` |
| `diff APP ID` | `GET …/candidates/{id}/diff` |
| `verify APP ID` | `POST …/candidates/{id}/verify` |
| `preview APP ID` | Existing candidate preview; kind-specific launch in H3 |
| `approve APP ID` | `POST …/candidates/{id}/approve` |
| `reject APP ID` | `POST …/candidates/{id}/reject` |
| `suggest APP` | `GET …/suggest` |
| `incident APP …` | Existing incident report/list operations |
| `history/undo/restore/release/offer/adopt APP …` | Thin calls to existing `src` operations |
| `submit APP REV [--story ID]` | New `POST /api/shape/{app}/submit`, using `src` PR/weave |

Every CLI verb supports `--json`; results carry the same meaningful fields as
the corresponding operation, with no HTML required to inspect a diff. Existing
web wire responses stay compatible. Usage, authorization, missing-resource,
conflict and verification errors return failure, never a success-shaped record.
A negative verifier verdict remains inspectable and has a nonzero CLI status.

Local `wish` waits for bounded generation and returns the resulting candidate.
It does not spawn an untracked background process. Web requests retain their
existing asynchronous running/ready behavior. `--from` copies/imports into an
isolated candidate workspace and never builds or edits the supplied checkout.
Both paths record the same base and candidate identities. A failed or interrupted
attempt remains inspectable; a retry creates a new candidate unless an existing
operation explicitly supports resumption. No retry implies approval.

Core states remain running → ready → verified → approved, with failure and
rejection paths. Approval reruns verification, imports the exact candidate
objects, and invokes the production activation gate, which rechecks sealed
evidence. A cached JSON verdict is not activation authority. Reject removes
only that candidate's disposable resources; already running work must not
recreate its metadata or activate afterward. Preserve the existing protected
page request/new-app behavior, draft sessions, suggestion and incident behavior.

## Identity, authorization and scope

- **Actor** is the accountable caller. **Represented user** is the principal
  whose app version is selected. The generating agent's identity and receipt
  are separate provenance; they never replace either caller field.
- Local CLI follows existing `src` selectors: `--user`, alias `--principal`,
  then `BASHY_PRINCIPAL`; an absent principal fails, without guessing an OS
  login. Actor resolution uses the existing authored-actor/session resolver
  (including its explicit `--as` behavior), not an arbitrary unchecked string.
- Local access retains the existing trusted OS-account boundary used by `src`:
  selecting a principal is not a newly issued delegation grant. Do not present
  local flags as authenticated remote impersonation or invent a new identity
  system. Preserve existing actor/session checks when an agent is detected.
- Web and remote access take caller identity and represented principal from
  the existing authenticated launcher context. Body fields, query parameters,
  and inbound principal headers cannot override that identity. Existing
  protected-part and operator-only checks continue to apply.
- Web apps activate for one principal. CLI apps activate for one host in a
  side-slot binary while retaining the release and PATH. Services activate for
  one host by switching the service image while preserving application data.
  Kind-specific verification and recovery belong to H3; they do not bypass
  the shared lifecycle. Other users or hosts retain their active versions.

Resolve registered app sources through the fleet catalog. Its public shaped
registration imports source into the managed `src` checkout; do not activate
against the original example directory. The console retains its existing
embedded-source seed path. History and activation must address the same home,
managed checkout and principal.

## Tracking and evidence

Every new wish/import creates and links one ordinary todo unless mutually
exclusive `--story ID` or `--todo ID` identifies an existing item. Validate the
link before generation; failure to persist tracking blocks generation. No
sprint is automatically required for an ordinary shaping todo.

The linked item carries label `shaping` and frontmatter `origin: shape`, with
app, candidate/version ID, actor and represented user. Candidate and tracking
records refer to each other. Reuse the existing issue store with only the
required metadata additions; do not write a separate shaping backlog. Preserve
unrelated metadata when linking an existing item.

The durable identity record links source/base commit, candidate commit,
workspace/branch, artifact identity, verifier evidence, app kind/activation
scope, actor and represented user. Before a build exists, artifact identity is
absent rather than fabricated. For a static web app the candidate tree/commit
is the served artifact identity; CLI binaries and service images additionally
record their built-artifact digest. Record the generating agent and receipt
when present. An approval may clean up a workspace only after durable version,
tracking and evidence links make the approved revision inspectable/submittable.

Submit uses the already tracked item unless `--story` explicitly links the
submission to a story. CLI and web submit create the same reviewable `src`
proposal/weave records. Submitted commits carry `Shape: APP@VERSION` and
`Shaped-By: ACTOR[ for REPRESENTED_USER]`, plus the normal provenance required
by the linked work. H4 extends the existing commit guard to validate the links.
Offer/adopt remains sharing; submit does not automatically merge or require a
Sprint 570 handoff. Existing review/merge gates remain the only merge gates.

## Concurrency, recovery and acceptance

The CLI introduces a second writer process. Use existing filesystem-lock
primitives for candidate transitions and app/principal activation, with an
app/repository lock for shared Git imports. In-memory console locks alone are
insufficient. Avoid holding a metadata lock during model generation: record
running state, release it, then reacquire and confirm the candidate still
exists and is running before publishing the result. Serialize final approval
against rejection, competing activation and stale-base changes. A changed base
requires fresh matching evidence, not reuse of an earlier pass. Interrupted
activation retains a recoverable prior version; failed checks never activate.

H2 parity tests exercise the shared service through web and CLI adapters,
including wrong-principal lookup, rejection, failed/stale evidence, interruption
and concurrent transitions. H3 verifies kind-specific build/activation/recovery.
H4 verifies identical submission identity/trailers and normal gates. H5 runs
its existing three-platform and console-regression requirements. H7 uses the
Hello World and self-shaping recipes with real selected agent interfaces.
Missing interfaces or evidence remain blocked, not passed. Existing Sprint 526
web behavior and stored principal histories remain compatible throughout.

## Inspection references

- `yoke/pkg/webconsole/shaped_candidate.go`: candidate lifecycle, isolated
  clones, verification and production activation currently live here.
- `yoke/pkg/webconsole/shaped_history.go` and `principal.go`: managed-source
  consistency and authenticated web-principal boundary.
- `yoke/pkg/src/cmd_history.go`, `activate.go` and `cmd.go`: principal selectors,
  version lines and existing source/version command surface.
- `yoke/pkg/fleet/apps.go` and `shaped_import.go`: public registration imports
  source into the managed store.
- `yoke/pkg/bus/actor.go` and `yoke/pkg/issue/issue.go`: current authored-actor
  resolution and ordinary tracking records; H1 does not replace either system.
