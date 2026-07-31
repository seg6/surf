SDK_DROP := $(abspath $(CURDIR)/../buildenv/sdk)
REPO_ROOT := $(abspath $(CURDIR)/../..)
VERSION := $(shell tr -d '[:space:]' < $(REPO_ROOT)/VERSION)
PROTOCOL_VERSION := $(shell tr -d '[:space:]' < $(REPO_ROOT)/PROTOCOL_VERSION)
$(shell cd $(CURDIR) && VERSION='$(VERSION)' PROTOCOL_VERSION='$(PROTOCOL_VERSION)' ./render-versioned-files.sh >/dev/null)

ifeq ($(wildcard $(SDK_DROP)/iPhoneOS8.0.sdk),)
$(error Missing $(SDK_DROP)/iPhoneOS8.0.sdk. Run native/buildenv/fetch-sdk.sh or see docs/native-build.md)
endif

TARGET := iphone:clang:8.0
TARGET_OS_DEPLOYMENT_VERSION_armv7 := 6.0
TARGET_OS_DEPLOYMENT_VERSION_arm64 := 7.0
ARCHS := armv7 arm64

include $(THEOS)/makefiles/common.mk

APPLICATION_NAME := Surf
TOOL_NAME := surf-update-v2
Surf_FILES := $(wildcard Classes/*.m) $(wildcard Classes/*.c)
# VideoToolbox is intentionally absent: private on iOS 6, resolved via dlopen.
Surf_FRAMEWORKS := UIKit CoreGraphics QuartzCore ImageIO Security CFNetwork AudioToolbox CoreMedia CoreVideo OpenGLES MessageUI
Surf_CFLAGS := -fobjc-arc -Wall -Werror=return-type -Wno-deprecated-declarations -DRBAppVersion='@"$(VERSION)"' -DRBNativeVersion='@"$(PROTOCOL_VERSION)"'
surf-update-v2_FILES := Updater/main.c
surf-update-v2_INSTALL_PATH := /usr/libexec
surf-update-v2_CFLAGS := -Wall -Werror

include $(THEOS_MAKE_PATH)/application.mk
include $(THEOS_MAKE_PATH)/tool.mk

after-stage::
	@mkdir -p $(THEOS_STAGING_DIR)/Applications/Surf.app
	@cp Resources/icon-57.png $(THEOS_STAGING_DIR)/Applications/Surf.app/icon-57.png
	@cp Resources/icon-57@2x.png $(THEOS_STAGING_DIR)/Applications/Surf.app/icon-57@2x.png
	@cp Resources/icon-72.png $(THEOS_STAGING_DIR)/Applications/Surf.app/icon-72.png
	@cp Resources/icon-72@2x.png $(THEOS_STAGING_DIR)/Applications/Surf.app/icon-72@2x.png
	@cp Resources/icon-144.png $(THEOS_STAGING_DIR)/Applications/Surf.app/icon-144.png
	@cp Resources/Default.png $(THEOS_STAGING_DIR)/Applications/Surf.app/Default.png
	@cp Resources/Default@2x.png $(THEOS_STAGING_DIR)/Applications/Surf.app/Default@2x.png
	@cp Resources/Default-568h@2x.png $(THEOS_STAGING_DIR)/Applications/Surf.app/Default-568h@2x.png
	@cp Resources/Default-667h@2x.png $(THEOS_STAGING_DIR)/Applications/Surf.app/Default-667h@2x.png
	@cp Resources/Default-736h@3x.png $(THEOS_STAGING_DIR)/Applications/Surf.app/Default-736h@3x.png
	@cp Resources/Default-Landscape-736h@3x.png $(THEOS_STAGING_DIR)/Applications/Surf.app/Default-Landscape-736h@3x.png
	@chmod 0755 $(THEOS_STAGING_DIR)/Applications/Surf.app/Surf
	@chown 0:0 $(THEOS_STAGING_DIR)/usr/libexec/surf-update-v2
	@chmod 4755 $(THEOS_STAGING_DIR)/usr/libexec/surf-update-v2

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
	@chmod 4755 $(THEOS_STAGING_DIR)/usr/libexec/surf-update-v2
