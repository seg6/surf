SDK_DROP := $(abspath $(CURDIR)/../buildenv/sdk)
REPO_ROOT := $(abspath $(CURDIR)/../..)
VERSION := $(shell tr -d '[:space:]' < $(REPO_ROOT)/VERSION)
PROTOCOL_VERSION := $(shell tr -d '[:space:]' < $(REPO_ROOT)/PROTOCOL_VERSION)
$(shell cd $(CURDIR) && VERSION='$(VERSION)' PROTOCOL_VERSION='$(PROTOCOL_VERSION)' ./render-versioned-files.sh >/dev/null)

ifeq ($(wildcard $(SDK_DROP)/iPhoneOS6.1.sdk),)
$(error Missing $(SDK_DROP)/iPhoneOS6.1.sdk. Run native/buildenv/fetch-sdk.sh or see docs/native-build.md)
endif

TARGET := iphone:clang:6.1:6.0
ARCHS := armv7

include $(THEOS)/makefiles/common.mk

APPLICATION_NAME := Surf
Surf_FILES := $(wildcard Classes/*.m) $(wildcard Classes/*.c)
# VideoToolbox is intentionally absent: private on iOS 6, resolved via dlopen.
Surf_FRAMEWORKS := UIKit CoreGraphics QuartzCore ImageIO Security CFNetwork AudioToolbox CoreMedia CoreVideo MessageUI
Surf_CFLAGS := -fobjc-arc -Wall -Werror=return-type -Wno-deprecated-declarations -DRBNativeVersion='@"$(PROTOCOL_VERSION)"'

include $(THEOS_MAKE_PATH)/application.mk

after-stage::
	@mkdir -p $(THEOS_STAGING_DIR)/Applications/Surf.app
	@cp Resources/icon-72.png $(THEOS_STAGING_DIR)/Applications/Surf.app/icon-72.png
	@cp Resources/icon-72@2x.png $(THEOS_STAGING_DIR)/Applications/Surf.app/icon-72@2x.png
	@cp Resources/icon-144.png $(THEOS_STAGING_DIR)/Applications/Surf.app/icon-144.png
	@chmod 0755 $(THEOS_STAGING_DIR)/Applications/Surf.app/Surf

# The buildenv container bind-mounts the repo (native/buildenv/build.sh),
# so Resources/* keep the host's numeric UID/GID in the staged bundle no
# matter what copies theos's own resource-staging step. On a real device
# that shows up as an app bundle owned by a UID that means nothing on iOS
# (not root, not mobile) — MobileInstallation's registration scan silently
# skips bundles like that instead of erroring, which looks exactly like a
# missing home-screen icon with zero diagnostics anywhere. Force sane
# ownership/permissions as the very last step before packaging so this
# can never regress silently again.
before-package::
	@chown -R 0:0 $(THEOS_STAGING_DIR)/Applications/Surf.app
	@find $(THEOS_STAGING_DIR)/Applications/Surf.app -type d -exec chmod 0755 {} +
	@find $(THEOS_STAGING_DIR)/Applications/Surf.app -type f -exec chmod 0644 {} +
	@chmod 0755 $(THEOS_STAGING_DIR)/Applications/Surf.app/Surf
