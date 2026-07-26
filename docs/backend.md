# Backend

The backend is named `surf-backend`. It runs Chromium inside Docker, captures
the X display with ffmpeg, and streams H.264/audio/control messages to the
native client.

## Local LAN Launcher

From `backend/`, `./start.sh` builds the Docker image, hashes required
`SURF_PASSWORD`, starts the container with host networking, and enables Bonjour
discovery. It refuses to start when `SURF_PASSWORD` is unset.

Configuration is read from `backend/.env`:

```sh
SURF_PASSWORD=change-me
PORT=18080
```

## Docker Compose

`backend/docker-compose.yml` defines a `surf-backend` service for deployments
where you want to provide `AUTH_HASH` yourself.

Generate a hash with:

```sh
cd backend
docker build -t surf-backend:lan .
docker run --rm --entrypoint /app/surf-backend surf-backend:lan -hash-password 'your-password'
```

## Important Ports

- `18080`: default LAN port used by `./start.sh`.
- `8080`: default internal container port in compose deployments.

## Security

Do not expose a weak-password backend to the public internet. For VPS usage,
put it behind HTTPS and use a strong password.
