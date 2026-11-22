# release-mirror

bashy as the LAN install server. One host (the organizer: a teacher's laptop, a
team's dev box) holds a mirror shaped exactly like the GitHub release download
base and serves it over plain HTTP. Everyone else on the LAN installs or
upgrades from it with one line, with or without bashy, on macOS, Windows or
Linux, and AI agents read the served page as their skill. Upstream releases and
the organizer's own builds (shaped, candidate, local) publish the same way.

    bashy release-mirror fetch                       # mirror the latest upstream release (all platforms)
    bashy release-mirror publish bin/ --id rc40-dev --latest   # or a dist assets dir / one archive
    bashy release-mirror serve                       # :8080; prints the one-liners
    bashy release-mirror index [--json]              # what is on the mirror (agents)

Participants (or their agent) are told one sentence:

> Set up bashy from http://<organizer-host>:8080

The served `README.md` is the live `bashy skill show bashy` plus these lines:

    curl -fsSL http://<organizer-host>:8080/install.sh | sh        # macOS / Linux
    irm http://<organizer-host>:8080/install.ps1 | iex             # Windows PowerShell

They are the outpost installers with `BASHY_RELEASE_BASE` pre-set to this
mirror: the archive and its `checksums.txt` come from the mirror (the
organizer host is the trust anchor on that LAN; `BASHY_RELEASE_CHECKSUMS=github`
keeps the upstream digests for release tags), then `bashy self install` runs
as always (atomic replace, downgrade refusal). An existing bashy is upgraded
in place by the same line.

Layout: `latest`, `<tag>/bashy-<os>-<arch>.tar.gz|.zip`, `<tag>/checksums.txt`,
`install.sh`, `install.ps1`, `README.md`. Default directory
`~/.bashy/release-mirror` (`BASHY_RELEASE_MIRROR`), repo `qiangli/bashy`
(`BASHY_RELEASE_REPO`). `publish` accepts a `dist/assets` directory from the
release build, a single archive, or a host build directory (`bin/` after
`bashy dag build`: it packs `bashy bash sh outpost` for this platform from the
Go payload, never the C launcher pair).

Register once from a stable checkout:

    bash examples/release-mirror/register.sh

`mcp` runs bashy's own MCP on loopback for agents that can reach the organizer
host (ssh forward or `outpost mesh service add bashy-mcp 127.0.0.1:8765`); the
LAN bind with a bearer token is a follow-up story. Serving uses
`bashy rclone serve http --read-only`; peers with outpost can expose the same
directory over the mesh as service `bashy-release`.
