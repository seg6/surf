# Docker deployment

The image downloads a pinned Surf Linux release, verifies its SHA-256 digest,
and runs `surf serve` with system Chromium.

Create `.env` beside the Compose file:

```dotenv
SURF_VERSION=0.9.0
SURF_ARCH=amd64
SURF_ARCHIVE_SHA256=replace-with-the-release-archive-sha256
SURF_PASSWORD=replace-me
```

Existing named volumes can be retained by setting `SURF_RUNTIME_VOLUME`,
`SURF_PROFILE_VOLUME`, `SURF_DOWNLOADS_VOLUME`, or `SURF_UPLOADS_VOLUME`.
Set `SURF_ARCH=arm64` on an ARM64 Docker host and use that release archive's
SHA-256 digest.

`SURF_ARCH` selects only the Linux backend executable for the Docker host. Each
release archive embeds the same universal rootful package containing armv7/iOS
6 and arm64/iOS 7–14 client slices for iPhone, iPod touch, and iPad.

Surf does not redistribute Widevine. DRM is available only when the browser
inside the container already provides a compatible CDM; the default distro
Chromium image should not be assumed to include one. Surf leaves browser
component/CDM loading enabled and reports the actual EME result in authenticated
`/health?stats=1` output.

Then deploy:

```sh
docker network create web 2>/dev/null || true
docker compose up -d --build
docker compose logs --tail=100 surf
```

The browser profile, downloads, uploads, and Surf-managed runtime are stored in
named volumes. The service is available as `surf:8080` on the external `web`
network; route it through the stack's reverse proxy. Pin the version, architecture,
and matching archive digest together when upgrading.
