# Install the desktop client

The Linux client connects to a Surf host with the same pairing and stream as
the iOS one. It works on X11 and Wayland. The
[desktop client notes](../clients/desktop.md) have more detail.

To run a release, extract the archive and start `bin/surf-client` from the
extracted folder:

```sh
tar xzf surf-desktop-<version>-linux-<arch>.tar.gz
cd surf-desktop-<version>-linux-<arch>
./bin/surf-client
```

The client takes no command-line options. The README inside the archive lists
the runtime libraries and how to install it system wide. The client keeps its
identity and preferences separate from the host's `SURF_HOME`.

To build from source on Debian or Ubuntu:

```sh
sudo apt-get install libasound2-dev libavcodec-dev libavformat-dev \
  libavutil-dev libgl1-mesa-dev libxkbcommon-x11-0
cargo run --manifest-path client/desktop/Cargo.toml -p surf-client
# or: make desktop-run
client/desktop/package-linux.sh   # local package
```

Start the [host](install-host.md), then either enter `host:port` or use
discovery, and [pair](pairing.md). Check that the six words match on both
screens. If you've saved only one server, the client reconnects to it on its
own.

The tray app and `surf serve` are the host. The desktop client is only a
remote UI and doesn't run a browser itself.
