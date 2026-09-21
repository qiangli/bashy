---
id: 1725e2fb706c
kind: feature
title: 'S227.5 Windows image: bashy.exe on nanoserver, same build-image target, proven on a real Windows container host'
seq: 309
status: blocked
priority: p1
labels:
    - windows
    - oci
created: 2026-09-20T21:23:20.212699Z
---

**DEFERRED — unlinked from sprint 227 (operator, 2026-09-20).** A Windows-native image needs the Containers feature (Pro/Enterprise/Education/Server) and Docker in Windows-containers mode — not the self-contained story and not the common Windows box. Sprint 227 covers Windows with S227.9 instead: the linux image through the self-contained `bashy podman` machine on WSL2 (works on Home). This story stays filed for a later sprint that has a Server/Pro container host and a reason to ship a native image.

bashy cross-compiles to windows/{amd64,arm64} with CGO_ENABLED=0 (no launcher pair — the launcher is unix-only), so the artifact exists; the image does not. Windows has no FROM scratch: the smallest base is `mcr.microsoft.com/windows/nanoserver:<ltsc>`.

Deliver: `build-image` with `BASHY_OCI_PLATFORM=windows/amd64` selects `tools/bashy-image/Containerfile.windows` — nanoserver base, `C:\bashy\bashy.exe`, `ENTRYPOINT ["C:\\bashy\\bashy.exe"]`, `BASHY_BIN_CACHE=C:\bashy\cache`, `PATH` = `C:\bashy;C:\Windows\System32`. Externals per S227.2 (MinGit is the git row here — `bashy git` downloads it on Windows, so it must be pre-seeded); the `$COMSPEC /c` pitfalls from the umbrella Windows punch-list apply inside the container too.

Engine (corrected at review, 2026-09-20): the lean `bashy podman` DOES build on Windows (`engines_stub.go` tag `!bashy_engines || (windows && …)`, with `applyPodmanHelperEnv` for the machine helpers) — but podman on Windows drives a WSL Linux machine and cannot build or run a Windows-native (nanoserver, process-isolated) image. That, not a build tag, is why this story's host engine is Docker in Windows-containers mode / containerd+hcsshim on the container host: the recorded exception to the sprint's engine rule.

Host edition (operator Q, 2026-09-20): Windows Home cannot be the gate host — the Containers feature (and Hyper-V) exists only on Pro / Enterprise / Education / Server; on Home, Docker Desktop and podman run Linux containers only. Verify the edition (`Get-ComputerInfo WindowsProductName`) and that `Containers` is enabled before naming the host here; process isolation also needs the host build to match the nanoserver ltsc tag.

Gate on a real Windows container host (process isolation; the existing Windows host from the v0.24.x QA lanes if it has the containers feature, else name the host that does) — cross-compile green is NOT evidence: the sibling Windows work found silent failures four times in one day. Run the S227.3 matrix rows there; rows that differ from linux get their own column.

Acceptance: `docker run --rm --network=none -v ${PWD}:C:\work -w C:\work localhost/bashy:…-windows-amd64 --bashsharp .\script.bsh` runs a .bsh with builtin coreutils and one island; size table row; host + isolation mode + ltsc tag recorded in the evidence.
