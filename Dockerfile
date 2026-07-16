# --- build: static Go binary (client assets are embedded via go:embed) ---
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/rbrowser ./cmd/rbrowser

# --- run: Chromium + Xvfb (headful mode is far less bot-flagged) ---
FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      chromium \
      xvfb \
      ffmpeg \
      ca-certificates \
      fonts-liberation fonts-noto-core fonts-noto-color-emoji \
      libnss3 libatk-bridge2.0-0 libatk1.0-0 libcups2 libdrm2 \
      libgtk-3-0 libgbm1 libasound2 libxdamage1 libxkbcommon0 libpango-1.0-0 \
      libxrandr2 libxcomposite1 libxfixes3 libxcursor1 libxi6 \
    && rm -rf /var/lib/apt/lists/*

ENV CHROME=/usr/bin/chromium \
    PROFILE=/data/profile

WORKDIR /app
COPY --from=build /out/rbrowser ./rbrowser
COPY start.sh ./
RUN chmod +x start.sh && mkdir -p /data/profile

EXPOSE 8080
CMD ["./start.sh"]
