FROM rust:1-bookworm

ENV CARGO_HOME=/opt/bashy-cargo-cache
COPY uutils.tar /tmp/uutils.tar
RUN mkdir -p /tmp/uutils \
 && tar -xf /tmp/uutils.tar -C /tmp/uutils \
 && cd /tmp/uutils \
 && cargo fetch --locked \
 && rm -rf /tmp/uutils /tmp/uutils.tar \
 && chmod -R a+rX /opt/bashy-cargo-cache \
 && touch /opt/bashy-uutils-image-v1
