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
make surf-binary
./backend/surf doctor
```

Surf installs managed Chrome and FFmpeg automatically. If a download is
blocked, permit access to Google Chrome for Testing storage and GitHub
Releases, or set `CHROME`/`FFMPEG` to explicit local executables. Linux audio
also requires the PulseAudio-compatible tools reported by `doctor`.

## Managed Runtime Download Fails

Retry with the default managed runtime settings:

```sh
make surf-binary
unset CHROME FFMPEG SURF_BROWSER_DOWNLOAD SURF_FFMPEG_DOWNLOAD
SURF_PASSWORD='change-me' ./backend/surf serve
```

Downloads are checksum-verified and stored below `SURF_HOME/runtime`. Set
`SURF_BROWSER_DOWNLOAD=0` or `SURF_FFMPEG_DOWNLOAD=0` only when provisioning
the corresponding runtime yourself.

## Browser Is Visible Or Frames Stop When Unfocused

The current backend always uses Chrome `--headless=new`; it does not use Xvfb
or desktop capture. A visible window means an old backend process or an
explicitly launched browser is still running. Stop all old `surf serve`
processes, rebuild, and start the current binary.

## App Installed But Icon Did Not Update

Run on the device:

```sh
uicache
```

If needed, respring.

## Logs

Backend:

```sh
SURF_PASSWORD='change-me' ./backend/surf serve
```

The backend runs in the foreground and writes logs to the terminal.

Native app:

```text
/var/mobile/Library/Surf/surf.log
```
