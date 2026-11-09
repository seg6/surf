# Install the desktop client

Linux Dear ImGui client that talks to a normal Surf host (same pairing and
stream as iOS). X11 is what the session tests cover; Wayland builds. Details:
[Clients: desktop](../clients/desktop.md).

**From a release**: follow the archive README. Client identity/preferences are
separate from the host's `SURF_HOME`.

**From source** (Debian/Ubuntu):

```sh
sudo apt-get install libasound2-dev libavcodec-dev libavformat-dev \
  libavutil-dev libgl1-mesa-dev libxkbcommon-x11-0
cargo run --manifest-path client/desktop/Cargo.toml -p surf-client
# or: make desktop-run
client/desktop/package-linux.sh   # local package
```

Start the [host](install-host.md), enter `host:port` or use discovery,
[pair](pairing.md), match the six words. One saved server reconnects on its own.

The tray/`surf serve` binary is the **host**. This program is only a remote UI.
