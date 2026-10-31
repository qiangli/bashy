---
id: 2b0913389feb
kind: bug
title: S530.6 bashy host podman engine wires yoke pkg/nfthelper so embedded netavark finds bashy's nft (S530.5 companion, bashy side)
seq: 3014
status: done
priority: p1
created: 2026-10-31T02:51:08.208661Z
weave: 402
assignee: codex-gpt6-sol
sprint: 530
sprint_id: 1d6b9478-c054-5b09-961e-c649a3bc1705
sprint_title: 'Master-provisioned AWS satellites: cloudbox provisions Lightsail satellites from owner-held AWS credentials (S522.5)'
closed: 2026-10-31T03:58:32.87671Z
closed_by: s530-manager
---

Companion to S530.5 (yoke ac23a2648237; yoke run 751 commit d820b289 adds pkg/nft JSON input + pkg/nfthelper). Operator 2026-10-30: real bashy bug, fixed in-sprint, does not gate S530.4.

Finding (S530.5 worker): released bashy (bashy_host) drives podman through bashy `internal/agentos/engines_stub.go` + `engines_podman.go`, not yoke `external/podman/engine/cli.go`, so the yoke helper alone cannot fix a bare Linux host.

Scope: bump bashy's yoke pin to the merged S530.5 SHA (conductor gives it); in `engines_podman.go` import `github.com/qiangli/yoke/pkg/nfthelper`, call `Ensure()` in `applyManagedPodmanEnv` before the podman exec, prepend the returned dir to the child PATH and set `nfthelper.EnvName=1`; put `nfthelper.Dir()` first in `podmanConfOverrideLinux` helper_binaries_dir. bashy's own command namespace must NOT gain a public `nft` (a host nft keeps winning in the user's shell). Red test first: a unit test that the managed podman env/conf carries the helper dir and env only for the embedded engine (and not with $BASHY_PODMAN_SYSTEM), then green. Gate: focused `go test ./internal/agentos/ -run <yours>`, `go build ./...`, `go vet`; full `make test` is the conductor's freeze step. Live proof (conductor, elephant WSL, no nft installed, bashy built from this tree): default-bridge `bashy podman run --rm alpine:3.22 wget -qO- https://proxy.golang.org`, `-p` port map, `network create` + named-network run + `network rm` all succeed.
