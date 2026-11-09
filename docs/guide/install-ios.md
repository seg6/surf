# Install on iOS

Rootful jailbreak package for iPhone / iPod touch / iPad, iOS 6.0-14.8.1.
Device matrix: [Clients: iOS](../clients/ios.md).

**Cydia / Sileo**: add source `https://seg6.space/surf/`, install Surf, respring
if the icon is missing. The site also has the current `.deb`.

**SSH:**

```sh
scp space.seg6.surf_*.deb root@DEVICE_IP:/tmp/surf.deb
ssh root@DEVICE_IP 'dpkg -i /tmp/surf.deb'
ssh root@DEVICE_IP 'su mobile -c uicache || uicache || true'
```

Run `uicache` as `mobile` on older jailbreaks if root fails.

Then start the [host](install-host.md), [pair](pairing.md), and allow TCP
`18080` (or your `PORT`) to the computer. Use the host LAN address.

**Updates:** Cydia/Sileo refresh, manual `.deb`, or the host's embedded package
when compatibility requires it (pinned download and hash checks;
[Security](../reference/security.md#host-data-and-updates)).
