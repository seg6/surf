# Security

Surf encrypts the connection between a client and its backend. The trust anchor
is the server identity stored in `SURF_HOME`.

## Server identity

First launch creates an RSA 2048 self signed certificate. Its SHA 256 leaf
fingerprint is the server ID. Clients save that fingerprint during pairing and
reject later changes.

The listener has no plaintext mode. Control messages, media, uploads,
downloads, diagnostics, and client updates use TLS. Old clients can negotiate
TLS 1.2 with ECDHE RSA. Newer peers may use TLS 1.3. Session resumption is
disabled so each transport presents the certificate.

Bonjour advertises an address and server ID. It does not replace certificate
pinning.

With `SURF_TUNNEL_HOST`, a Cloudflare WebSocket carries a separate Surf TLS
connection. Cloudflare can see connection metadata and encrypted traffic
volume, but cannot read the Surf connection.

## Pairing

Pairing is closed until **Pair device** or `surf pair` creates an invitation.
An invitation closes after one device uses it, after cancellation, after a
server restart, or after five wrong manual codes.

QR pairing carries the endpoint, a random 128 bit token, and part of the
expected certificate fingerprint. The client checks the presented certificate
before saving its full fingerprint.

Manual pairing uses an address and six digit code. The host and client then
show six words derived from the certificate and client public key. Pairing must
be cancelled when the words differ. The numeric code authorizes the request.
The words confirm that both ends saw the same server identity.

## Devices and sessions

Each client creates a separate RSA 2048 key for every server. The private key is
stored as a `ThisDeviceOnly` Keychain item. The backend stores its public key.

Authentication signs a fresh 30 second challenge bound to API v1, the server
ID, device ID, challenge ID, and a random nonce. A successful authentication
returns an HTTP session and a device bound, single use WebSocket ticket.

Revoking a device removes it, invalidates its challenges and tickets, and
closes its connections.

## Clipboard

Clipboard settings are available only through the local administration
interface. Clipboard text travels through the authenticated Surf connection.

Surf stores the sync setting, not clipboard contents. Text is not passed as a
command argument or written to logs. A one time value expires after two minutes
if it has not changed.

Operating systems and other local software may read text while it remains on a
system clipboard.

## Host data

`SURF_HOME` contains the TLS private key, session key, paired public keys,
browser profile, and browsing data. A copy of the directory is a copy of the
server identity. Replacing it causes saved clients to report **Server Identity
Changed**.

## Updates

The backend offers the matching iOS package over an authenticated and pinned
connection. The client checks its length and SHA 256 hash. The privileged
helper checks the hash, package ID, version, architecture, and input path before
installation.

This blocks network substitution. It does not protect against a compromised
host or stolen server identity, since either can replace the package and its
advertised hash.

## Boundary

Surf encryption ends at the backend. Chromium handles website TLS and holds
decoded pages, media, credentials, cookies, and downloads. Control of the host,
browser profile, or `SURF_HOME` grants access to that data.

The unauthenticated `/api/v1/health` and `/api/v1/server` endpoints return
reachability and server metadata. Browser configuration, media, files,
statistics, updates, and WebSockets require a paired device.
