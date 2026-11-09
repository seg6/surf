# Remote access

By default Surf binds TLS on all interfaces at port `18080` and expects clients
on the same LAN. For access over the internet you need a reachable address and
matching pairing codes. Surf does not implement NAT traversal by itself.

## Public address

Put the address clients should dial into pairing codes:

```sh
SURF_PUBLIC_ADDRESS=surf.example.net:18080 ./surf serve
```

Open that TCP port on the host firewall or VPS security group. A public CA
certificate is not required; clients pin Surf's self-signed certificate.

## Cloudflare Tunnel

Route a hostname to the Surf HTTPS listener and set both variables:

```sh
SURF_PUBLIC_ADDRESS=surf-roam.example.net:443 \
SURF_TUNNEL_HOST=surf-roam.example.net \
./surf serve
```

The public WebSocket carries a **separate** pinned Surf TLS connection inside
the tunnel. Cloudflare can see connection metadata and encrypted traffic
volume. It cannot read Surf control or media plaintext. A direct LAN endpoint
still gives lower latency for local use.

Details: [Security](../reference/security.md).

## Discovery

Bonjour/DNS-SD can advertise a LAN address and server ID. It does not replace
certificate pinning. When multicast discovery fails, enter the host address
manually.

## Checklist

1. Host running and healthy (`surf status`, `/api/v1/health`).
2. Firewall allows the Surf port end to end.
3. `SURF_PUBLIC_ADDRESS` matches what the client can resolve and reach.
4. Pair (or re-pair) so the client stores the correct address and fingerprint.
5. Prefer LAN when both devices are local.
