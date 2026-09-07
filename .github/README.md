# CI and build caches

CI always runs the C99, Go, Rust, desktop interaction/session, and native
package checks. A cache hit reuses compiler dependencies; it never substitutes
for tests or skips compiling the current source.

## Rust desktop

Debug tests use `client/desktop/target`. Release packaging uses
`.local/build/desktop-release`. They have separate `rust-cache` entries so a
failed debug test cannot leave an incomplete cache that prevents saving the
release dependencies later. Both save compiled dependencies even when a later
test fails.

The release cache uses the same workspace mapping and shared key in CI and
the release workflow. Verified main builds warm it before a signed release tag
builds the archive. Both jobs use Ubuntu 24.04 and Rust 1.95.0; the cache action
also keys by compiler, build environment, and Cargo manifests/lockfile. Bump
the shared key in **both workflows** when changing the native library ABI or
runner distribution.

GitHub scopes PR caches to their PR merge ref. Main/tag release builds do not
consume a PR's cache; their initial build may still be cold. Lockfile changes
can restore compatible dependency artifacts, but changed dependencies and Surf
source still rebuild. Debug and optimized release code require separate builds.

## Native iOS

The downloaded, verified SDK has its own cache keyed by its fetch script.
Buildx caches the pinned Theos/toolchain Docker layers in GitHub's cache with
scope `native-buildenv`. Native source and package verification still run on
every build. The release workflow separately retains its trusted, content-keyed
GHCR build-environment image.

## Go

CI uses `setup-go` caching with `backend/go.sum`. The release workflow keeps
its separate cross-platform Go module/build cache, keyed by Go version,
dependency checksum, and verified source commit with a dependency-prefix
fallback.

## Headless integration

The desktop job explicitly installs X11 keyboard support, software OpenGL,
Xvfb, ripgrep, and PulseAudio. Its clocked null audio sink allows the real ALSA
playback path to run without a physical sound card.

The session test checks complete frame delivery, input, audio, reconnect,
resize, bounded queues, and UI-stall recovery. FPS is reported; a fixed FPS
gate is opt-in on controlled hardware with `SURF_TEST_MIN_FPS=55`. Shared
software-rendered runners are functional test hosts, not reference benchmark
machines.

Updated action revisions are pinned to commit SHAs with their versions in
comments. Checkout, cache, and Docker JavaScript actions use Node 24.
