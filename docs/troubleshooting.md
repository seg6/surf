# Troubleshooting

## Surf Cannot Connect To LAN Backend

Check the backend health endpoint from another device on the same network:

```text
http://YOUR_COMPUTER_IP:18080/health
```

If local health works but the iOS device times out, your computer firewall is
probably blocking inbound TCP `18080`.

For `ufw`:

```sh
sudo ufw allow from 192.168.0.0/16 to any port 18080 proto tcp
```

## Wrong Password

Restart the backend with the intended `SURF_PASSWORD`, then reconnect from Surf.

## Backend Tool Check Fails

Run:

```sh
make backend-binary
./backend/surf-backend doctor
```

Install any missing required tools listed by the doctor output.

## Chromium Window Is Visible

Rebuild the backend and make sure you are not overriding the private display:

```sh
make backend-binary
unset SURF_MANAGE_DISPLAY SURF_DISPLAY
SURF_PASSWORD='change-me' ./backend/surf-backend serve
```

On Wayland desktops, Surf should still run Chromium inside private Xvfb. A
visible host window usually means an old binary is running or the display was
explicitly overridden.

## App Installed But Icon Did Not Update

Run on the device:

```sh
uicache
```

If needed, respring.

## Logs

Backend:

```sh
SURF_PASSWORD='change-me' ./backend/surf-backend serve
```

The backend runs in the foreground and writes logs to the terminal.

Native app:

```text
/var/mobile/Library/Surf/surf.log
```
