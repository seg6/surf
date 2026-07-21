# --- build: static Go binary ---
FROM golang:1.26-bookworm AS build
ARG PROTOCOL_VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X surf-backend/internal/config.NativeVersion=${PROTOCOL_VERSION}" -o /out/surf-backend ./cmd/surf-backend

# --- run: Chromium + Xvfb (headful mode is far less bot-flagged) ---
FROM debian:bookworm-slim
ARG VERSION=dev
ARG PROTOCOL_VERSION=dev
LABEL org.opencontainers.image.title="surf-backend" \
      org.opencontainers.image.version="${VERSION}" \
      space.seg6.surf.protocol-version="${PROTOCOL_VERSION}"

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      chromium \
      xvfb \
      x11-xserver-utils \
      ffmpeg \
      pulseaudio \
      ca-certificates \
      fonts-liberation fonts-noto-core fonts-noto-color-emoji \
      libnss3 libatk-bridge2.0-0 libatk1.0-0 libcups2 libdrm2 \
      libgtk-3-0 libgbm1 libasound2 libxdamage1 libxkbcommon0 libpango-1.0-0 \
      libxrandr2 libxcomposite1 libxfixes3 libxcursor1 libxi6 \
    && rm -rf /var/lib/apt/lists/*

ENV CHROME=/usr/bin/chromium \
    PROFILE=/data/profile

WORKDIR /app
COPY --from=build /out/surf-backend ./surf-backend
COPY docker-entrypoint.sh ./
RUN chmod +x docker-entrypoint.sh && mkdir -p /data/profile

EXPOSE 8080
CMD ["./docker-entrypoint.sh"]
