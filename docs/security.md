# Security

Surf encrypts and authenticates the connection between a client and its
backend without requiring a public certificate or reverse proxy. Its trust
anchor is the server identity created under `SURF_HOME`.

## Server identity and transport

On first launch, Surf creates a persistent RSA-2048 self-signed certificate.
Its SHA-256 leaf fingerprint is the server ID. Paired clients pin that exact
fingerprint instead of using the device's public CA store, which keeps TLS 1.2
usable on iOS 6 and fails closed if the identity later changes.

The direct listener has no plaintext mode. Configuration, control messages,
video, audio, metadata, uploads, downloads, diagnostics, and client updates
travel over TLS. TLS 1.2 uses ECDHE-RSA suites supported by iOS 6; modern peers
may negotiate TLS 1.3. Session resumption is disabled so every new transport
presents the pinned certificate.

Bonjour advertises an address and server ID but is only a locator. A spoofed
advertisement cannot satisfy an existing certificate pin.

When `SURF_TUNNEL_HOST` is configured, Cloudflare terminates only the outer
public WebSocket connection. That WebSocket transports an independent Surf TLS
connection whose certificate is checked against the existing server pin.
Cloudflare can observe connection metadata and encrypted byte volume, but not
Surf control messages, media, credentials, browsing data, or client updates.

## Pairing

Pairing is closed until the server owner chooses **Pair device** or runs
`surf pair`. One invitation accepts one device and closes after use,
cancellation, server restart, or five incorrect manual codes.

- QR pairing carries the endpoint, a random 128-bit one-time token, and a
  128-bit prefix of the expected certificate fingerprint. The client verifies
  that prefix against the presented certificate and saves the full observed
  SHA-256 pin.
- Manual pairing uses the address and six-digit authorization code, followed
  by a six-word comparison derived independently from the certificate and the
  client's public key. The words must match on the server and client.

The six words are important: a short numeric code authorizes one attempt, but
by itself cannot prove that an active relay did not substitute a different
self-signed endpoint. QR carries that identity out of band; manual pairing
uses the word comparison instead. Cancel immediately if the words differ.

## Device authentication and revocation

The client creates a separate RSA-2048 key for each server. Its private key is
stored as a `ThisDeviceOnly` Keychain item; the backend stores only the public
key. Each authentication signs a fresh 30-second challenge bound to API v1,
the server ID, device ID, challenge ID, and random nonce.

Successful authentication issues a Secure, HttpOnly, SameSite session and a
device-bound, single-use WebSocket ticket. Revocation removes the device,
invalidates its outstanding challenges and tickets, and closes its active
connections immediately.

Host-to-device clipboard delivery is an owner-only loopback admin operation.
The text travels inside the paired device's pinned TLS/WebSocket session; Surf
does not place it in command-line arguments, logs, or host storage. The native
client clears the delivered value after two minutes if its clipboard has not
changed. While present, it has the normal iOS clipboard trust boundary and may
be readable by other software running on that device.

Protect `SURF_HOME`. It contains the TLS private key, session-signing key,
paired public keys, browser profile, and browsing data. Copying the directory
copies the server identity; deleting or replacing it intentionally makes saved
clients report **Server Identity Changed**.

## Updates

The matching native `.deb` is offered only over an authenticated, pinned Surf
connection. The client verifies the advertised byte length and SHA-256, and
the privileged helper rechecks the hash plus package ID, version, architecture,
and safe input path before installation.

This protects the package against network substitution. It is not an
independent publisher-signature system: a compromise of the trusted backend or
its server identity can also replace the package and its advertised hash.

## Trust boundary

Surf is end-to-end encrypted between the device and the Surf backend, not
between the device and a website. Chromium runs on the backend, where website
TLS terminates and decoded pages, media, credentials, cookies, and downloads
exist. Anyone who controls that host, its browser profile, or `SURF_HOME` is
inside Surf's trust boundary.

The unauthenticated `/api/v1/health` and `/api/v1/server` endpoints expose only
reachability and server metadata. New pairing requests are accepted only while
an owner-created invitation is active. Browser configuration, media, files,
statistics, updates, and WebSockets require a paired device.
