.PHONY: test surf-binary surf-dist native-sdk native-package

VERSION := $(shell tr -d '[:space:]' < VERSION)
PROTOCOL_VERSION := $(shell tr -d '[:space:]' < PROTOCOL_VERSION)
SURF_GOOS ?= $(shell go env GOOS)
SURF_GOARCH ?= $(shell go env GOARCH)
CLIENT_DEB ?=
BUNDLE_FFMPEG ?= 0
empty :=
space := $(empty) $(empty)
comma := ,
SURF_DIST := surf-$(VERSION)-$(SURF_GOOS)-$(SURF_GOARCH)
ifeq ($(SURF_GOOS),windows)
SURF_EXE := surf.exe
SURF_ARCHIVE := $(SURF_DIST).zip
else
SURF_EXE := surf
SURF_ARCHIVE := $(SURF_DIST).tar.gz
endif

test:
	cd backend && go test ./...

# The tray dependency uses native desktop APIs. Build on the target OS; CI uses
# native Linux, Windows, and macOS runners.
surf-binary:
	if [ -n "$(CLIENT_DEB)" ]; then \
		test -f "$(CLIENT_DEB)"; \
		cp "$(CLIENT_DEB)" backend/internal/clientupdate/bundle/client.deb; \
	fi
	if [ "$(BUNDLE_FFMPEG)" = "1" ]; then \
		cd backend && go run ./tools/fetchffmpeg internal/ffmpegbin/bundle/ffmpeg; \
	fi
	if [ "$(SURF_GOOS)" = "windows" ]; then \
		cd backend/cmd/surf-desktop && go run github.com/tc-hib/go-winres@v0.3.1 simply \
			--arch "$(SURF_GOARCH)" --out rsrc --manifest gui --icon surf-icon.png \
			--product-version "$(VERSION).0" --file-version "$(VERSION).0" \
			--file-description "Surf remote browser backend" --product-name Surf \
			--original-filename surf.exe; \
	fi
	cd backend && CGO_ENABLED=1 GOOS=$(SURF_GOOS) GOARCH=$(SURF_GOARCH) go build -trimpath \
		$(if $(strip $(if $(CLIENT_DEB),client_bundle) $(if $(filter 1,$(BUNDLE_FFMPEG)),ffmpeg_bundle)),-tags="$(subst $(space),$(comma),$(strip $(if $(CLIENT_DEB),client_bundle) $(if $(filter 1,$(BUNDLE_FFMPEG)),ffmpeg_bundle)))") \
		-ldflags="-s -w $(if $(filter windows,$(SURF_GOOS)),-H=windowsgui) -X surf-backend/internal/config.AppVersion=$(VERSION) -X surf-backend/internal/config.NativeVersion=$(PROTOCOL_VERSION)" \
		-o "$(SURF_EXE)" ./cmd/surf-desktop

surf-dist: BUNDLE_FFMPEG=1
surf-dist: surf-binary
	rm -rf "dist/$(SURF_DIST)" "dist/Surf.app"
	if [ "$(SURF_GOOS)" = "darwin" ]; then \
		mkdir -p "dist/Surf.app/Contents/MacOS"; \
		cp "backend/$(SURF_EXE)" "dist/Surf.app/Contents/MacOS/surf"; \
		mkdir -p "dist/Surf.app/Contents/Resources"; \
		cp packaging/desktop/THIRD_PARTY_NOTICES.md "dist/Surf.app/Contents/Resources/THIRD_PARTY_NOTICES.md"; \
		rm -rf "dist/Surf.iconset"; mkdir -p "dist/Surf.iconset"; \
		for size in 16 32 128 256 512; do \
			sips -z $$size $$size backend/cmd/surf-desktop/surf-icon.png --out "dist/Surf.iconset/icon_$${size}x$${size}.png" >/dev/null; \
			double=$$((size * 2)); sips -z $$double $$double backend/cmd/surf-desktop/surf-icon.png --out "dist/Surf.iconset/icon_$${size}x$${size}@2x.png" >/dev/null; \
		done; \
		iconutil -c icns "dist/Surf.iconset" -o "dist/Surf.app/Contents/Resources/Surf.icns"; \
		rm -rf "dist/Surf.iconset"; \
		sed "s/@VERSION@/$(VERSION)/g" packaging/desktop/Info.plist > "dist/Surf.app/Contents/Info.plist"; \
		tar -C dist -czf "dist/$(SURF_ARCHIVE)" "Surf.app"; \
		hdiutil create -volname Surf -srcfolder "dist/Surf.app" -ov -format UDZO "dist/surf-$(VERSION)-darwin-$(SURF_GOARCH).dmg" >/dev/null; \
	else \
		mkdir -p "dist/$(SURF_DIST)"; \
		cp "backend/$(SURF_EXE)" "dist/$(SURF_DIST)/$(SURF_EXE)"; \
		cp packaging/desktop/README.md "dist/$(SURF_DIST)/README.md"; \
		cp packaging/desktop/THIRD_PARTY_NOTICES.md "dist/$(SURF_DIST)/THIRD_PARTY_NOTICES.md"; \
		if [ "$(SURF_GOOS)" = "linux" ]; then cp packaging/backend/surf.service "dist/$(SURF_DIST)/surf.service"; fi; \
		if [ "$(SURF_GOOS)" = "windows" ]; then \
			rm -f "dist/$(SURF_ARCHIVE)"; \
			cd backend && go run ./tools/zipdir "../dist/$(SURF_DIST)" "../dist/$(SURF_ARCHIVE)"; \
			if command -v iscc >/dev/null 2>&1; then \
				sed -e "s|@VERSION@|$(VERSION)|g" -e "s|@SOURCE@|$(CURDIR)/dist/$(SURF_DIST)|g" \
					-e "s|@OUTPUT_DIR@|$(CURDIR)/dist|g" packaging/windows/surf.iss.in > dist/surf.iss; \
				iscc dist/surf.iss; \
				rm dist/surf.iss; \
			fi; \
		else \
			tar -C dist -czf "dist/$(SURF_ARCHIVE)" "$(SURF_DIST)"; \
			if [ -n "$${APPIMAGETOOL:-}" ]; then \
				rm -rf dist/Surf.AppDir; \
				mkdir -p dist/Surf.AppDir/usr/bin dist/Surf.AppDir/usr/share/doc/surf; \
				cp backend/surf dist/Surf.AppDir/usr/bin/surf; \
				cp packaging/desktop/THIRD_PARTY_NOTICES.md dist/Surf.AppDir/usr/share/doc/surf/THIRD_PARTY_NOTICES.md; \
				cp packaging/desktop/surf.desktop dist/Surf.AppDir/surf.desktop; \
				cp backend/cmd/surf-desktop/surf-icon.png dist/Surf.AppDir/surf.png; \
				ln -s usr/bin/surf dist/Surf.AppDir/AppRun; \
				ARCH=x86_64 "$$APPIMAGETOOL" --appimage-extract-and-run dist/Surf.AppDir "dist/surf-$(VERSION)-linux-amd64.AppImage"; \
			fi; \
		fi; \
	fi

native-sdk:
	native/buildenv/fetch-sdk.sh

native-package: native-sdk
	docker run --rm --network host -v "$(CURDIR):/src" surf-buildenv make -C /src/native/client clean package DEBUG=0
