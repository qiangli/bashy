# Install bashy

Every way to install bashy. The short version is on the [README](../README.md); an AI agent can do all of it for you — tell it *"Set up bashy from https://github.com/qiangli/bashy"*.

Setting up a new machine end to end (install, pairing, with or without an AI
agent, locally or from another machine): see [docs/setup.md](setup.md).

## Install bashy

The installers fetch the current stable release, verify its archive against
`checksums.txt`, then run `bashy self install`.

**macOS or Linux (curl):**

```sh
curl -fsSL https://dhnt.io/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://dhnt.io/install.ps1 | iex
```

Both install scripts show no operating-system prompt.

**Package managers are not yet available.** Homebrew, Scoop and winget
channels are **not supported** yet: there is no `dhnt/tap`
formula, no `dhnt` Scoop bucket and no winget package to install from. Use an
install script above or a release archive below.

The v1.0 macOS and Windows binaries are not yet code-signed. A browser-downloaded
archive prompts at launch: on macOS, right-click the executable and choose
**Open**, or remove quarantine from it with
`xattr -d com.apple.quarantine <file>`; on Windows, choose **More info → Run
anyway**, or unblock the downloaded file with `Unblock-File <file>`. DHNT Inc.
signed builds arrive in v1.0.x.

**Linux tarball:** download the matching `bashy-linux-amd64.tar.gz` or
`bashy-linux-arm64.tar.gz` from the
[latest release](https://github.com/qiangli/bashy/releases/latest), then
verify and install it (this example selects the host architecture):

```sh
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) echo "unsupported architecture" >&2; exit 1 ;; esac
asset="bashy-linux-$arch.tar.gz"
curl -fSLO "https://github.com/qiangli/bashy/releases/latest/download/$asset"
curl -fsSLO https://github.com/qiangli/bashy/releases/latest/download/checksums.txt
grep "  $asset\$" checksums.txt | sha256sum -c -
tar -xzf "$asset"
./bashy self install --dir "$HOME/.local/bin"
```

Starting with v1.0, Linux releases also publish `.deb`, `.rpm`, and `.apk`
packages alongside the tarballs. macOS and Windows archive names are `bashy-darwin-{amd64,arm64}.tar.gz`
and `bashy-windows-{amd64,arm64}.zip`.

`latest` always names the current stable release ([releases/latest](https://github.com/qiangli/bashy/releases/latest)). A candidate prerelease (a
`-rc.N-dev` tag) is never `latest`: download one from its tag,
`https://github.com/qiangli/bashy/releases/download/<tag>/<asset>`. Release
notes: [`releases/`](releases/) and the [CHANGELOG](../CHANGELOG.md).

## With Go

`go install github.com/qiangli/bashy@latest` is **not** supported: the module
uses `replace` directives for its forks (the `mvdan.cc/sh/v3` engine and a
few vendored upstreams), which `go install` refuses. Use a release archive above, or build
from source below.

## From source

bashy rebuilds itself using only an installed bashy. Every command below is
run *through* `bashy`, so the same five lines work on Linux, macOS and
Windows: `bashy git` fetches the sources, and `bashy dag build` compiles both binaries through `bashy go`, which downloads
and verifies its own pinned Go toolchain into bashy's cache the first time.
The sibling modules are go.mod pins, so the go command downloads exactly the
versions this commit names.

```sh
bashy git clone https://github.com/qiangli/bashy
cd bashy
bashy dag build            # -> bin/bash and bin/bashy (bin/*.exe on Windows)
bashy dag install          # optional: install into ~/.local/bin ($DHNT_BIN_DIR to change)
```

What the host must provide, per platform:

| | git | Go | C compiler |
| --- | --- | --- | --- |
| Windows | **none** — `bashy git` downloads a pinned, checksum-verified MinGit | **none** — `bashy go` provisions it | not used |
| macOS | the system `git` (Xcode Command Line Tools: `xcode-select --install`) | **none** — `bashy go` provisions it | optional |
| Linux | the distribution's `git` | **none** — `bashy go` provisions it | optional |

The C compiler is optional on Linux and macOS: with `cc` on `PATH` the build
also compiles the native pre-Go signal launcher (`bin/bashy` + `bin/bashy.real`);
without one it says so and ships the plain Go binaries — the same form the
release archives ship. `bashy git` on Linux and macOS deliberately uses the
platform git rather than downloading one.

A checkout that has no installed bashy yet can bootstrap from the repo-local
launcher instead (Linux/macOS; it needs a host `go`):

```sh
./bashy dag build
./bashy dag install
```

The traditional host-tool path also works when `git`, `go` and `make` are
already installed:

```sh
git clone https://github.com/qiangli/bashy
cd bashy
make build                         # -> bin/bash and bin/bashy
```

## Containers — the offline image

Bashy is all you need. With nothing but the release download — no git, go,
podman or docker on the host — build the image and run your script with no
network:

```sh
bashy self image
bashy podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:<ver>-linux-<arch> --bashsharp ./script.bsh
```

`bashy self image` fetches the release's static `bashy-scratch-linux-<arch>`
artifact (checksum-verified) and builds a `FROM scratch` image around it
through `bashy podman` — the engine bashy provisions for itself from the
pinned upstream releases (a complete static podman on Linux; the machine
client plus gvproxy/vfkit on macOS; the client on Windows, where the machine
runs on WSL2 — every edition). The image is bashy as it is: Bash 5.3, `--posix`,
Bash#, the builtin coreutils, dag/weave/check/transpile. What is and is not in
it, per command, is the measured matrix in
[`docs/airgap-image.md`](airgap-image.md). From a checkout,
`bashy dag build-image` images the candidate instead of a published artifact.

A minimal Linux container base (Ubuntu/glibc, launcher + payload) is described
in [`docs/bashy-oci-base.md`](bashy-oci-base.md).

Real-repository examples driven by `bashy dag` — one `dag.md` each for
GitHub CLI, Hugo, Caddy, curl, git, FFmpeg, tesseract, llama.cpp, CMake, uv,
Codex, Bun, OpenCode, OpenClaw, Hermes Agent and more, calling their Go,
Python, TypeScript, Rust and C/C++ code as fenced islands — live under
[`examples/dag/`](../examples/dag/).
