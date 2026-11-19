# CI and build caches

There are three workflows:

| Workflow | Trigger | What it does |
| --- | --- | --- |
| Continuous Integration | PRs, and pushes to `main` | Tests and verifies. It never publishes. |
| Release | Manual `workflow_dispatch` with a signed tag | Builds the host and client assets and publishes the GitHub release |
| Package Repository | Manual, and queued by Release | Builds the Astro site and apt metadata and pushes `gh-pages` |

CI always runs the C99, Go, Rust, desktop interaction and session, native
package and documentation site checks. A cache hit only reuses compiled
dependencies. It never skips the tests or the build of the current source. All
CI jobs run on Ubuntu 24.04.

## Rust desktop

Debug tests use `client/desktop/target`, and release packaging uses
`.local/build/desktop-release`. Each has its own `rust-cache` entry. That way a
failed debug test can't leave behind an incomplete cache that stops the release
dependencies from being saved later. Both entries save compiled dependencies
even if a later test fails.

`cache-bin: false` keeps rustup-managed tools out of cache cleanup. Without it,
the first post-job cache save can remove `cargo` before the second cache gets
saved. The pinned setup action installs the toolchains, so they aren't restored
from the dependency cache.

CI and the release workflow use the same workspace mapping and shared key for
the release cache. Release only runs when it's dispatched by hand for a signed
tag, so the first release after a cache key bump may still be cold. Both jobs
use Ubuntu 24.04 and Rust 1.95.0, and the cache action also keys on the
compiler, the build environment and the Cargo manifests and lockfile. If you
change the native library ABI or the runner distribution, bump the shared key
in both workflows.

GitHub scopes PR caches to the PR's merge ref, so manual release builds never
use a PR's cache. A lockfile change can still restore compatible dependency
artifacts, but changed dependencies and Surf's own source get rebuilt. Debug
and optimized release code need separate builds.

## Native iOS

The downloaded and verified SDK has its own cache, keyed by its fetch script.
Buildx caches the pinned Theos and toolchain Docker layers in GitHub's cache
under the scope `native-buildenv`. The native source and package verification
still run on every build. The release workflow separately keeps its own trusted,
content-keyed build-environment image on GHCR.

## Go

The visible browser-setup tests run as part of `go test ./...`, with private
Chromium profiles and Xvfb displays. CI installs Xvfb and uses its bundled
Chrome. If either is missing, local runs skip these tests. They cover the
tab and profile handoff, the standalone CLI's lifetime, rollback, stale
confirmations and recovery from an explicit force close. They never open
windows on your desktop.

CI uses `setup-go` caching keyed on `backend/go.sum`. The release workflow keeps
a separate cross-platform Go module and build cache, keyed by the Go version,
the dependency checksum and the verified source commit, with a fallback to the
dependency prefix.

## Documentation site

CI builds the Astro site in `site/` and rejects any CSS or JS that iOS 6 WebKit
can't use: flex, grid, custom properties, `@font-face`, unprefixed
`linear-gradient`, and script tags in the static output.

## Headless integration

The desktop job installs X11 keyboard support, software OpenGL, Xvfb, ripgrep
and PulseAudio. Its clocked null audio sink lets the real ALSA playback path
run without a physical sound card.

The session test checks full frame delivery, input, audio, reconnecting,
resizing, bounded queues and recovery from a UI stall. It reports FPS, but a
fixed FPS requirement is opt-in for controlled hardware, using
`SURF_TEST_MIN_FPS=55`. The shared software-rendered runners are good enough to
test function, but they aren't benchmark machines.

When an action is updated, its revision is pinned to a commit SHA with the
version in a comment. The checkout, cache and Docker JavaScript actions use
Node 24.
