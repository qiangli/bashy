# syntax=docker/dockerfile:1
#
# bench.Containerfile — the bashy-vs-GNU head-to-head measurement image.
#
# Holds BOTH arms side-by-side in ONE Linux image so every comparison has zero
# cross-machine / kernel / filesystem variance (the macOS dev host ships BSD
# tools, so a "beat GNU coreutils 9.11" number can only be produced here):
#
#   - GNU arm  : GNU coreutils 9.11 + GNU bash 5.3, built from source into /opt/gnu
#                (Debian only ships coreutils 9.1, so we pin 9.11 ourselves).
#   - bashy arm: the linux `bashy` binary at /usr/local/bin/bashy
#                (in-process coreutils via `bashy <tool>`).
#
# perfbench (coreutils/cmds/perfbench) drives both:
#   GNU arm   →  $GNU_PREFIX/bin/<tool>
#   bashy arm →  $BASHY_BIN <tool>   (cold spawn)  /  in-process tool.Run  /  warm `bashy serve`
#
# Build (from the repo root, after `cd bashy && make dist` produces bashy-linux-<arch>):
#   cp bashy/dist/bashy-linux-arm64 bashy/eval/agent-shell/containers/
#   podman build --build-arg TARGETARCH=arm64 \
#     -f bashy/eval/agent-shell/containers/bench.Containerfile \
#     -t bashy-bench:9.11 bashy/eval/agent-shell/containers/
#
# The GNU program set built here is the COMPLETE coreutils src/ inventory
# (github.com/coreutils/coreutils/tree/master/src): --enable-install-program adds
# `arch` and `hostname`, the two programs coreutils omits from the default install,
# so /opt/gnu/bin holds every command perfbench's inventory (inventory.go) expects.

ARG COREUTILS_VERSION=9.11
ARG BASH_VERSION=5.3

##############################################################################
# Stage 1 — build GNU coreutils 9.11 + GNU bash 5.3 from source into /opt/gnu
##############################################################################
FROM docker.io/library/debian:bookworm-slim AS build
ARG COREUTILS_VERSION
ARG BASH_VERSION

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      build-essential wget xz-utils ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# --- GNU coreutils ${COREUTILS_VERSION} ---
# SUPPLY-CHAIN PIN (do before trusting any published number): fetch the release
# and verify its checksum against ftp.gnu.org/gnu/coreutils/coreutils-${VER}.tar.xz.sig
# (or the sha256 the release announcement publishes). Replace the bare wget with a
# checksum-checked download once the value is recorded.
RUN wget -q "https://ftp.gnu.org/gnu/coreutils/coreutils-${COREUTILS_VERSION}.tar.xz" \
 && tar -xf "coreutils-${COREUTILS_VERSION}.tar.xz"
RUN cd "coreutils-${COREUTILS_VERSION}" \
 && FORCE_UNSAFE_CONFIGURE=1 ./configure --prefix=/opt/gnu \
      --enable-install-program=arch,hostname \
      --without-selinux \
 && make -j"$(nproc)" \
 && make install

# --- GNU bash ${BASH_VERSION} ---
RUN wget -q "https://ftp.gnu.org/gnu/bash/bash-${BASH_VERSION}.tar.gz" \
 && tar -xf "bash-${BASH_VERSION}.tar.gz"
RUN cd "bash-${BASH_VERSION}" \
 && ./configure --prefix=/opt/gnu --without-bash-malloc \
 && make -j"$(nproc)" \
 && make install

##############################################################################
# Stage 2 — the bench image: GNU arm (/opt/gnu) + bashy arm (/usr/local/bin)
##############################################################################
FROM docker.io/library/debian:bookworm-slim
ARG COREUTILS_VERSION
ARG BASH_VERSION
# buildx sets TARGETARCH (arm64|amd64); matches the `make dist` artifact name.
ARG TARGETARCH=arm64

COPY --from=build /opt/gnu /opt/gnu

# GNU extension tools (grep/sed/gawk/findutils) are separate packages, not
# coreutils — install Debian's GNU builds and expose them under $GNU_PREFIX so
# the perfbench GNU arm (which execs $GNU_PREFIX/bin/<cmd>) can reach them.
RUN apt-get update \
 && apt-get install -y --no-install-recommends grep sed gawk findutils \
 && rm -rf /var/lib/apt/lists/* \
 && for t in grep sed gawk find xargs; do ln -sf "$(command -v $t)" /opt/gnu/bin/$t; done \
 && ln -sf /opt/gnu/bin/gawk /opt/gnu/bin/awk

# Both linux binaries (staged next to this Containerfile before building):
#   - perfbench: the measurement HARNESS — links cmds/all, runs the GNU-exec +
#     in-process arms. This is what you invoke: `perfbench conformance` / `run`.
#   - bashy    : the shell — only the cold/warm perf arms exec it.
COPY perfbench-linux-${TARGETARCH} /usr/local/bin/perfbench
COPY bashy-linux-${TARGETARCH} /usr/local/bin/bashy
RUN chmod +x /usr/local/bin/perfbench /usr/local/bin/bashy

# Determinism controls shared by BOTH arms (the coreutils LC_ALL=C contract).
ENV LC_ALL=C \
    LANG=C \
    TZ=UTC \
    GNU_PREFIX=/opt/gnu \
    BASHY_BIN=/usr/local/bin/bashy \
    PATH=/opt/gnu/bin:/usr/local/bin:/usr/bin:/bin

# Record the EXACT reference versions into the image — every result file cites these,
# so a stored baseline always names its GNU coreutils / bash / bashy revisions.
RUN /opt/gnu/bin/sort --version | head -1 > /etc/gnu-coreutils-version \
 && /opt/gnu/bin/bash --version | head -1 > /etc/gnu-bash-version \
 && ( /usr/local/bin/bashy --version 2>/dev/null | head -1 > /etc/bashy-version || true )

WORKDIR /work
# Default: the harness. `podman run bashy-bench:9.11 list` prints the inventory;
# `... gen && ... conformance` and `... run` produce the baselines.
ENTRYPOINT ["/usr/local/bin/perfbench"]
