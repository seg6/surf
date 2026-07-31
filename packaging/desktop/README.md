# Desktop package

The desktop Surf executable owns one supervised backend process and a
loopback-only Settings page. The page shows stream health and logs, changes the
server name/public address/port, creates single-use pairing codes and QR
invitations, shows the active verification phrase, lists paired devices, and
revokes devices immediately.

The backend listener itself is pinned TLS. The local Settings page proxies only
loopback admin operations through the daemon's permission-restricted per-run
control descriptor, so device management is neither exposed nor authorized on
the LAN.

Platform packages preserve `SURF_HOME` across updates. That directory contains
the long-lived server identity and pairing registry; deleting it intentionally
creates a new server identity which existing clients will reject until they
forget and pair again.
