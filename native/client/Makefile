SDK_DROP := $(abspath $(CURDIR)/../buildenv/sdk)

ifeq ($(wildcard $(SDK_DROP)/iPhoneOS6.1.sdk),)
$(error Missing $(SDK_DROP)/iPhoneOS6.1.sdk. See native/buildenv/README.md)
endif

TARGET := iphone:clang:6.1:6.0
ARCHS := armv7

include $(THEOS)/makefiles/common.mk

APPLICATION_NAME := WRP
WRP_FILES := $(wildcard Classes/*.m)
WRP_FRAMEWORKS := UIKit CoreGraphics QuartzCore ImageIO Security
WRP_CFLAGS := -fobjc-arc -Wall -Werror=return-type -Wno-deprecated-declarations

include $(THEOS_MAKE_PATH)/application.mk

after-stage::
	@mkdir -p $(THEOS_STAGING_DIR)/Applications/WRP.app
	@cp ../../public/icon-72.png $(THEOS_STAGING_DIR)/Applications/WRP.app/icon-72.png
	@cp ../../public/icon-144.png $(THEOS_STAGING_DIR)/Applications/WRP.app/icon-144.png
	@chmod 0755 $(THEOS_STAGING_DIR)/Applications/WRP.app/WRP
