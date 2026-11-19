# HTTP API

The API lives at `/api/v1` on Surf's TLS listener. There is no plaintext HTTP.

| Class | Requirement |
| --- | --- |
| Public | Reachability and pairing bootstrap |
| Device session | Cookie obtained after a challenge |
| WebSocket | Single-use, device-bound ticket |
| Admin | Loopback plus the control token from `daemon.json` |

`/api/v1/health` and `/api/v1/server` don't need authentication, since they
only report reachability and metadata. Everything else, meaning browser, media,
files, stats, updates and WebSockets, needs a paired device (or admin, where
noted).

## Public and pairing

| Method | Path | Role |
| --- | --- | --- |
| GET | `/api/v1/health` | Liveness |
| GET | `/api/v1/server` | Server metadata for discovery and UI |
| GET | `/api/v1/tunnel` | Tunnel helper when `SURF_TUNNEL_HOST` is set |
| POST | `/api/v1/pairing/request` | Start or continue a pairing attempt |
| GET | `/api/v1/pairing/status/{id}` | Pairing status |
| POST | `/api/v1/pairing/confirm/{id}` | Client confirmation |
| POST | `/api/v1/pairing/cancel/{id}` | Cancel |
| POST | `/api/v1/pairing/ack/{id}` | Acknowledge completion |

Pairing stays closed until the owner opens an invitation on the host.

## Authentication

| Method | Path | Role |
| --- | --- | --- |
| POST | `/api/v1/auth/challenge` | Fresh 30s challenge |
| POST | `/api/v1/auth/complete` | Exchange a signature for an HTTP session |
| POST | `/api/v1/auth/revoke` | Device self-revoke (gated) |

Challenges bind API v1, the server ID, the device ID, the challenge ID and a
nonce. A successful authentication returns a session cookie (`surf_device`,
24h) that later device requests use.

## Session device routes

| Method | Path | Role |
| --- | --- | --- |
| GET | `/api/v1/config` | Capabilities, compatibility and client configuration |
| GET | `/api/v1/ws` | WebSocket upgrade with single-use ticket |
| GET, POST | `/api/v1/browser-mode` | Setup status and resume (device) |
| POST | `/api/v1/client/logs` | Upload bounded client log snapshots |
| any | `/api/v1/tab-icons/*` | Favicon assets |
| POST | `/api/v1/uploads` | File chooser uploads |
| any | `/api/v1/downloads/*` | List and fetch downloads (GET and HEAD, ranges, ETags) |
| GET | `/api/v1/updates/client` | Embedded iOS package when present |

```sh
curl -k https://127.0.0.1:18080/api/v1/health
```

## Admin (loopback)

Admin routes sit under `/api/v1/admin/...` and need the control token.

| Area | Examples |
| --- | --- |
| Pairing | Create, get and delete a session; list and reject candidates |
| Devices | List and revoke |
| Logs | Sources, fetch, stream, refresh, clear |
| Clipboard | Get and set text; enable and disable sync |
| Browser | GET status; POST `open` and `resume` with a revision |
| Config and shutdown | Local dashboard operations |

The admin API is meant for the tray, the dashboard and the `surf` CLI on the
same machine. It isn't meant for remote administration over the public port.

## WebSocket

After authenticating, the client gets a short-lived ticket and connects to
`/api/v1/ws`. Text frames carry control JSON, with commands going in and events
coming out. Binary frames carry `RBR1` media.

See [Protocol](protocol.md).
