# Build

From the repository root.

## Version injection

| File | Role |
| --- | --- |
| `VERSION` | App version (`MAJOR.MINOR.PATCH`) |
| `COMPATIBILITY_VERSION` | Network compatibility generation |

Release builds pass both into the Go binary via linker flags.

## Common targets

| Target | Result |
| --- | --- |
| `make test` | C core tests, Go tests, desktop Rust tests |
| `make client-core-test` | CMake build + ctest for `client/core` |
| `make client-core-sanitize` | ASan/UBSan core build + tests |
| `make desktop-build` / `desktop-test` / `desktop-run` | Linux client |
| `make surf-binary` | Host binary for current `GOOS`/`GOARCH` |
| `make surf-dist` | Packaged host archive for this machine |
| `make surf-release-dist` | Release desktop archives (needs tooling) |
| `make native-package` | Universal iOS `.deb` via buildenv |

`CLIENT_DEB` may point at a verified iOS package. When set, `surf-binary`
embeds it for client updates (`-tags=client_bundle`).

```sh
make surf-binary
make surf-dist
```

```sh
APPIMAGETOOL=/path/to/appimagetool make surf-release-dist \
  CLIENT_DEB=path/to/space.seg6.surf_VERSION_iphoneos-arm.deb
```

Linux AppImage builds need `appimagetool`. Windows installer builds need
`makensis`. macOS `.app` archives can be produced without Xcode or a Mac host.

## Layout of outputs

Build artifacts commonly land under `.local/build/` and `dist/`. Do not commit
them.

## Next

- [Native iOS build](native-ios.md)
- [Testing](testing.md)
- [Releasing](releasing.md)
