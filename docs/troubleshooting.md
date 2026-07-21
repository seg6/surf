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

Edit `backend/.env`, run `./start.sh` from `backend/`, then reconnect from Surf.

## App Installed But Icon Did Not Update

Run on the device:

```sh
uicache
```

If needed, respring.

## Logs

Backend:

```sh
docker logs surf-backend-lan
```

Native app:

```text
/var/mobile/Library/Surf/surf.log
```
