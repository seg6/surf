# Install on iOS

Surf for iOS is a package for rootful jailbreaks, and it supports iPhone, iPod
touch and iPad on iOS 6.0 to 14.8.1. The [iOS client notes](../clients/ios.md)
list the devices.

The simplest way is through Cydia or Sileo. Add `https://seg6.space/surf/` as a
source, install Surf, and respring if the icon doesn't show up. The site also
has the current `.deb` for installing it manually.

To install over SSH:

```sh
scp space.seg6.surf_*.deb root@DEVICE_IP:/tmp/surf.deb
ssh root@DEVICE_IP 'dpkg -i /tmp/surf.deb'
ssh root@DEVICE_IP 'su mobile -c uicache || uicache || true'
```

On older jailbreaks, run `uicache` as `mobile` if running it as root fails.

Next, start the [host](install-host.md) and [pair](pairing.md). The device
needs to reach the computer on TCP port `18080` (or whatever you set `PORT` to),
so allow that through the firewall and use the host's LAN address.

You can update through Cydia or Sileo, by installing a new `.deb` yourself, or
through the host's embedded package when the client and host versions no
longer match. That download is pinned and its hash is checked, which
[security](../reference/security.md#updates) explains.
