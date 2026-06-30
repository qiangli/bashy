FROM docker.io/library/debian:bookworm-slim

COPY bashy-linux-arm64 /usr/local/bin/bashy

RUN chmod +x /usr/local/bin/bashy \
  && ln -sf /usr/local/bin/bashy /usr/local/bin/bash \
  && ln -sf /usr/local/bin/bashy /bin/bash \
  && ln -sf /usr/local/bin/bashy /bin/sh

ENV DHNT_AGENT=1
ENV BASHY_ADVISOR=1
ENV SHELL=/usr/local/bin/bashy
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/bashy"]

