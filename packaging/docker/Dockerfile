FROM debian:bookworm-slim

ARG SURF_VERSION
ARG SURF_ARCH=amd64
ARG SURF_ARCHIVE_SHA256

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
      chromium \
      curl \
      fonts-liberation \
      fonts-noto-color-emoji \
      fonts-noto-core \
      pulseaudio \
      pulseaudio-utils \
    && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    archive="/tmp/surf.tar.gz"; \
    curl -fL "https://github.com/seg6/surf/releases/download/v${SURF_VERSION}/surf-${SURF_VERSION}-linux-${SURF_ARCH}.tar.gz" \
      -o "$archive"; \
    echo "${SURF_ARCHIVE_SHA256}  ${archive}" | sha256sum -c -; \
    mkdir -p /tmp/surf-release; \
    tar -xzf "$archive" -C /tmp/surf-release; \
    install -m 0755 "/tmp/surf-release/surf-${SURF_VERSION}-linux-${SURF_ARCH}/surf" /usr/local/bin/surf; \
    rm -rf "$archive" /tmp/surf-release

ENV SURF_HOME=/data/surf \
    PROFILE=/data/profile \
    DOWNLOADS=/data/downloads \
    UPLOADS=/data/uploads \
    CHROME=/usr/bin/chromium \
    PORT=8080

RUN mkdir -p /data/surf /data/profile /data/downloads /data/uploads

VOLUME ["/data/surf", "/data/profile", "/data/downloads", "/data/uploads"]
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/surf", "serve"]
