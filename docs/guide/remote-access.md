# Remote access

By default Surf listens for TLS on all interfaces at port `18080` and expects
clients on the same LAN. To use it over the internet you need an address that
clients can reach, and pairing codes that contain it. Surf doesn't do NAT
traversal on its own.

## Public address

Set the address clients should connect to, and it goes into the pairing codes:

```sh
SURF_PUBLIC_ADDRESS=surf.example.net:18080 ./surf serve
```

Open that TCP port on the host's firewall or your VPS security group. You don't
need a certificate from a public CA, because clients pin Surf's self-signed
one.

## Cloudflare Tunnel

Route a hostname to Surf's HTTPS listener and set both variables:

```sh
SURF_PUBLIC_ADDRESS=surf-roam.example.net:443 \
SURF_TUNNEL_HOST=surf-roam.example.net \
./surf serve
```

The public WebSocket carries a separate pinned Surf TLS connection inside the
tunnel. Cloudflare can see connection metadata and how much encrypted traffic
there is, but it can't read Surf's control or media data. A direct LAN
connection still has lower latency, so prefer it when you're at home.

More in [security](../reference/security.md).

## Discovery

Bonjour (DNS-SD) can advertise a LAN address and the server ID, but it doesn't
replace certificate pinning. If multicast discovery doesn't work on your
network, enter the host address by hand.

## Checklist

1. The host is running and healthy (`surf status`, `/api/v1/health`).
2. The firewall lets the Surf port through end to end.
3. `SURF_PUBLIC_ADDRESS` matches an address the client can resolve and reach.
4. Pair again if needed, so the client stores the right address and
   fingerprint.
5. Use the LAN when both devices are local.
