# Build

Run everything here from the repository root.

## Versions

Two files at the root control versioning. `VERSION` holds the app version
(`MAJOR.MINOR.PATCH`), and `COMPATIBILITY_VERSION` holds the network
compatibility generation. Release builds pass both into the Go binary through
linker flags.

## Make targets

| Target | What it does |
| --- | --- |
| `make test` | Runs the C core tests, the Go tests and the desktop Rust tests |
| `make client-core-test` | Builds `client/core` with CMake and runs ctest |
| `make client-core-sanitize` | Does the same build with ASan and UBSan on |
| `make desktop-build`, `desktop-test`, `desktop-run` | Build, test and run the Linux client |
| `make surf-binary` | Builds the host binary for the current `GOOS` and `GOARCH` |
| `make surf-dist` | Packages the host archive for this machine |
| `make surf-release-dist` | Builds the release desktop archives (needs extra tooling) |
| `make native-package` | Builds the universal iOS `.deb` through the build environment |

If you set `CLIENT_DEB` to a verified iOS package, `surf-binary` embeds it so
the host can offer it as a client update (this uses `-tags=client_bundle`).

```sh
make surf-binary
make surf-dist
```

```sh
APPIMAGETOOL=/path/to/appimagetool make surf-release-dist \
  CLIENT_DEB=path/to/space.seg6.surf_VERSION_iphoneos-arm.deb
```

The Linux AppImage build needs `appimagetool`, and the Windows installer needs
`makensis`. You can build the macOS `.app` archives without Xcode or a Mac.

## Where things end up

Build output usually lands in `.local/build/` and `dist/`. Don't commit either
one.

## Next

- [Native iOS build](native-ios.md)
- [Testing](testing.md)
- [Releasing](releasing.md)
