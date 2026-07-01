FROM docker.io/library/debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates build-essential autoconf bison make \
  && rm -rf /var/lib/apt/lists/*

COPY bash-5.3 /tmp/bash-5.3

RUN cd /tmp/bash-5.3 \
  && find . \( -name '*.o' -o -name '*.a' -o -name mksignames -o -name mkbuiltins \) -delete \
  && ./configure --prefix=/usr/local --without-bash-malloc \
  && make -j"$(nproc)" bash \
  && install -m 0755 bash /usr/local/bin/bash \
  && rm -rf /tmp/bash-5.3 \
  && ln -sf /usr/local/bin/bash /bin/bash \
  && ln -sf /usr/local/bin/bash /bin/sh

ENV BASHY_ADVISOR=0
ENV SHELL=/usr/local/bin/bash
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/bash"]
