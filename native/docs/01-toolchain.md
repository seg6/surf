# 01 — Toolchain: building armv7/iOS 6.0 Objective-C in 2026

Goal: a **Docker build environment** (`native/buildenv/`) that any agent can run on any machine to turn `native/client/` into an installable, pseudo-signed `.deb`. No dependence on the host's Xcode — modern Xcode cannot produce armv7/iOS 6 binaries at all (no armv7 slice support since ~Xcode 14, min deployment ~iOS 12).

**Host note:** the dev machine is macOS 26 (Apple Silicon). The container is not a "Linux requirement" — it runs under Docker Desktop / OrbStack on that Mac (`--platform linux/arm64` preferred; amd64 via Rosetta also works — pick whichever cctools-port pin builds cleanly and record it). The container exists because the *toolchain* has to be frozen somewhere modern Xcode can't reach, and because agents can then run the exact same build the user does.

## 1. The build environment image

`native/buildenv/Dockerfile`, Debian-based, containing:

| Component | What | Source |
|---|---|---|
| clang | open-source LLVM clang (distro package is fine, ≥14) — upstream clang still targets `armv7-apple-ios6.0` | `apt install clang lld` |
| cctools-port | Apple `ld`, `lipo`, `otool`, `strip` cross-built for Linux | github.com/tpoechtrager/cctools-port (pin a known-good commit) |
| ldid | pseudo-signing (SHA1 CodeDirectory is fine for iOS 6 AMFI-patched kernels) | github.com/sbingner/ldid or distro `ldid` |
| theos | build system, `iphone/application` template, .deb packaging | github.com/theos/theos (Linux install is first-class) |
| iPhoneOS6.1.sdk | headers + libs + frameworks | **user-provided**, see §2 |
| libarclite_iphoneos.a | ARC runtime shim for deployment targets < iOS 7 | **user-provided**, same archive as the SDK, see §3 |
| dpkg-deb, fakeroot, perl, git | packaging plumbing | apt |

Build entrypoint: `docker run --rm -v <repo>:/src wrp-buildenv make -C /src/native/client package` → emits `native/client/packages/*.deb`.

Theos project settings (`native/client/Makefile`):

```make
TARGET := iphone:clang:6.1:6.0     # SDK 6.1, deployment target 6.0
ARCHS := armv7
APPLICATION_NAME := WRP
WRP_FILES := $(wildcard Classes/*.m)
WRP_FRAMEWORKS := UIKit CoreGraphics QuartzCore CFNetwork Security \
                  ImageIO MobileCoreServices AudioToolbox OpenGLES CoreVideo CoreMedia
WRP_PRIVATE_FRAMEWORKS := VideoToolbox    # phase 3 only; guard behind a make flag until then
WRP_CFLAGS := -fobjc-arc -Wall -Werror=return-type
include $(THEOS)/makefiles/common.mk
include $(THEOS_MAKE_PATH)/application.mk
```

Verification commands the agent must run and record after every buildenv change:

```sh
lipo -info .theos/obj/WRP            # → "armv7"
otool -l .theos/obj/WRP | grep -A2 LC_VERSION_MIN_IPHONEOS   # → version 6.0
ldid -e .theos/obj/WRP               # entitlements blob present (may be empty dict)
dpkg-deb -c packages/*.deb           # → Applications/WRP.app/{WRP,Info.plist,icons}
```

## 2. Getting the iOS 6.1 SDK (user task, one-time)

The SDK is Apple-copyrighted, so it is **not** committed. `native/buildenv/sdk/` is gitignored; the build fails with a clear message if `sdk/iPhoneOS6.1.sdk/` is absent.

Options, in order of preference:
1. **Xcode 4.6.3 dmg** from developer.apple.com/download/all/ (needs any Apple ID; old Xcodes are still listed). Extract without installing:
   `Xcode.app/Contents/Developer/Platforms/iPhoneOS.platform/Developer/SDKs/iPhoneOS6.1.sdk` → copy into `native/buildenv/sdk/`.
   On a modern Mac the dmg mounts fine; the .app just won't run (don't need it to).
2. Community SDK archives (e.g. the well-known `iOS-SDKs` GitHub mirrors). Same directory lands in the same place.

Also extract from the *same* Xcode: `.../usr/lib/arc/libarclite_iphoneos.a` → `native/buildenv/sdk/libarclite_iphoneos.a`.

## 3. ARC vs MRC (risk R1)

We want ARC. Deployment target < iOS 7 requires `libarclite` for a handful of runtime entry points; modern toolchains no longer ship it.

- **Primary path:** vendor `libarclite_iphoneos.a` (§2) and add `-force_load $(SDK_ROOT)/libarclite_iphoneos.a` (or `-larclite_iphoneos` with `-L`) to LDFLAGS.
- **Phase 0 gate includes an ARC smoke test**: an app that allocates in loops, uses `__weak` (legal on iOS 5+), blocks capturing self, autorelease pools, container literals `@[] @{}` — runs 60s on device without crash or leak growth.
- **Fallback (record in PLAN.md decision log if taken):** compile MRC (`-fno-objc-arc`) with strict rules — every `alloc`/`copy`/`retain` paired in `dealloc`, properties `retain`/`assign` only, no blocks capturing self without a `__block` dance. Agents writing MRC must run the mental "who releases this" check in every method; prefer autorelease at creation.

Blocks, object literals, and subscripting all work fine against the 6.1 SDK either way.

## 4. Signing and packaging

Jailbroken iOS 6 (AMFI patched) runs `ldid`-fakesigned binaries — no certificates, no provisioning, no AppSync needed for /Applications-installed apps. Theos runs ldid automatically for `iphone:` targets.

Package as a .deb (`control` file: `Package: space.seg6.wrp`, `Section: Applications`, `Depends:` empty — no Cydia deps). Installs to `/Applications/WRP.app` (outside the sandbox container model — normal for jailbreak apps; gives us unrestricted `NSLog`/file access, which we exploit for logging).

`Resources/Info.plist` essentials:

```xml
CFBundleIdentifier      space.seg6.wrp
CFBundleExecutable      WRP
CFBundleDisplayName     WRP
UIDeviceFamily          <array><integer>2</integer></array>       <!-- iPad only -->
UISupportedInterfaceOrientations
                        LandscapeLeft + LandscapeRight            <!-- v1 landscape-only -->
UIStatusBarHidden       true
CFBundleIconFiles       icon-72.png, icon-144.png                 <!-- from tools/icon -->
UIPrerenderedIcon       true                                      <!-- no iOS-6 gloss on our flat icon -->
MinimumOSVersion        6.0
```

Icons: extend `tools/icon/main.go` to also emit the exact sizes into `native/client/Resources/` — same graphite/gold mark as the web app.

## 5. Installing on the iPad (user loop)

One-time device setup: install **OpenSSH** from Cydia; change both passwords (`passwd`, `passwd mobile`) from `alpine`; note the Wi-Fi IP. Optionally add the iPad to `~/.ssh/config` as `ipad`.

Per-build install:

```sh
scp packages/space.seg6.wrp_*.deb ipad:/tmp/
ssh ipad "dpkg -i /tmp/space.seg6.wrp_*.deb && uicache"
```

(`theos install` with `THEOS_DEVICE_IP` does the same; the scp form works from outside the container without theos on the host.)

`uicache` refreshes SpringBoard's app registry; no respring needed on iOS 6. Reinstalls over the top are fine.

## 6. Debugging on a 2013 device

No modern lldb attach. The loop is logs + overlay + crash reports:

1. **In-app log:** `RBLog(...)` writes timestamped lines to `/var/mobile/Library/WRP/wrp.log` (rotating at 1MB) *and* NSLog. Retrieve: `ssh ipad cat /var/mobile/Library/WRP/wrp.log`.
2. **Debug overlay** (port of the web client's): fps, decode ms, RTT, WS state, resident memory (`task_info`), toggled by triple-tap in a corner. This earned its keep in the web client; build it in Phase 1, not later.
3. **Crash reports:** `NSSetUncaughtExceptionHandler` + `signal()` handlers append reason + `[NSThread callStackSymbols]`/backtrace to the log before dying. Native crash plists land in `/var/mobile/Library/Logs/CrashReporter/` — scp them off. Keep the **unstripped** binary of every installed build (theos `.theos/obj/WRP` before strip; archive per version) so addresses can be symbolized on the Mac with `atos -arch armv7 -o WRP.unstripped -l <load addr>` or `llvm-symbolizer`.
4. **Console spew:** if needed, Cydia's syslog package mirrors NSLog to `/var/log/syslog`; usually the in-app log suffices.

## 7. Phase 0 deliverables & gates

Agent delivers:
- `native/buildenv/Dockerfile` + `README` (SDK drop-in instructions, exact build command)
- `native/client/` theos skeleton: Makefile, control, Info.plist, `Classes/RBAppDelegate.{h,m}`, a root view controller showing a label + the ARC smoke test button, RBLog, exception handlers
- `.gitignore` entries: `native/buildenv/sdk/`, `native/client/.theos/`, `native/client/packages/`
- Verification transcript (§1 commands)

User then runs §5 once. **Phase 0 done when the label renders on the iPad and the ARC smoke test survives 60s.**
