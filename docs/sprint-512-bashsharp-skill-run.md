# Sprint 512 / Story 2820: executable text-outer skills

A skill with Bash# fences and no skill.dhnt now runs through bashy's existing
LangBashPP interpreter and wireExec decorator/runtime wiring. Yoke owns fence
extraction, receipts, repair proposals and host overlays; bashy supplies the
runner through WithBashSharpRunner. Existing skill.dhnt execution is unchanged.

Implementation plan: add the host runner option in yoke, test receipts and
contract-preserving repairs with fake runners, commit yoke, wire bashy's runner,
bump both dependency pins, and prove the real interpreter plus go-repo-health
in a small module. All implementation steps are complete.

Yoke commit: 7ff2424c3bcef932ae7fad11b0e8da8ed72aa62c.

Validation on Dragon, 2026-10-28:

- `go test ./pkg/skills/...`: PASS (2.384s), including existing dhnt tests.
- `go vet ./pkg/skills/...`: PASS.
- `GOWORK=off go test -short ./internal/agentos -run
  '^TestSkillRunBashSharpContract$' -count=1 -v`: PASS (1.455s).
  Passing @ensure exits 0; failing @ensure exits 3, reports postcondition
  failure, and returns a failed receipt. Body output goes to stderr.
- `GOWORK=off go build -o /tmp/s512-bin/bashy ./cmd/bashy`: PASS.
- Installed binary, in `/tmp/s512-go-fixture`, with the umbrella skill catalog:
  exit 1, `has no skill.dhnt` (RED).
- Scratch binary, same directory/catalog: exit 0 (GREEN).
  `ok example.com/s512-health 0.255s`, then cached test success from @ensure.
  Receipt: valid=true, status=0, effects=[read,exec,net,write],
  identity=sha256:2f68cf96a8a705cf8d79f34e208ccb16452cc1d85c75ecac077cde069ed5f3c9.
  Receipt store: `/tmp/s512-live-skill-store`.

No binaries were installed; no full test suites, podman, or browser changes.
The manager owns the typeform-fill live fixture gate.

The worker contract prohibits pushing. To exercise the exact new dependency
pin with GOWORK=off, a temporary file GOPROXY was generated from `git archive`
of the committed yoke tree using golang.org/x/mod/zip.CreateFromDir. No local
replace was added to go.mod. The new go.sum entries describe that exact tree.
The conductor must publish that yoke commit before consumers can download
this pin from the remote. Local proxy: `/tmp/s512-proxy`; bashy checkout:
`/tmp/s512-bashy-issue-655`. The source pin and go.mod pin move together.
