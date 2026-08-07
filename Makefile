.PHONY: test surf-binary surf-dist surf-release-dist native-sdk native-package

VERSION := $(shell tr -d '[:space:]' < VERSION)
PROTOCOL_VERSION := $(shell tr -d '[:space:]' < PROTOCOL_VERSION)
SURF_GOOS ?= $(shell go env GOOS)
SURF_GOARCH ?= $(shell go env GOARCH)
CLIENT_DEB ?=
MAKENSIS ?= makensis
SURF_DIST := surf-$(VERSION)-$(SURF_GOOS)-$(SURF_GOARCH)
SURF_CGO_ENV := CGO_ENABLED=0
ifeq ($(SURF_GOOS),windows)
SURF_EXE := surf.exe
SURF_ARCHIVE := $(SURF_DIST).zip
else
SURF_EXE := surf
SURF_ARCHIVE := $(SURF_DIST).tar.gz
endif

test:
	cd backend && go test ./...

# Surf's tray implementation binds native desktop APIs without cgo, so every
# supported target cross-compiles from the same Go toolchain.
surf-binary:
	if [ -n "$(CLIENT_DEB)" ]; then \
		test -f "$(CLIENT_DEB)"; \
		cp "$(CLIENT_DEB)" backend/internal/web/client/client.deb; \
	fi
	if [ "$(SURF_GOOS)" = "windows" ]; then \
		cd backend/cmd/surf && go run github.com/tc-hib/go-winres@v0.3.1 simply \
			--arch "$(SURF_GOARCH)" --out rsrc --manifest gui --icon surf-icon.png \
			--product-version "$(VERSION).0" --file-version "$(VERSION).0" \
			--file-description "Surf remote browser backend" --product-name Surf \
			--original-filename surf.exe; \
	fi
	cd backend && $(SURF_CGO_ENV) GOOS=$(SURF_GOOS) GOARCH=$(SURF_GOARCH) go build -trimpath \
		$(if $(CLIENT_DEB),-tags=client_bundle) \
		-ldflags="-s -w $(if $(filter windows,$(SURF_GOOS)),-H=windowsgui) -X surf-backend/internal/config.AppVersion=$(VERSION) -X surf-backend/internal/config.NativeVersion=$(PROTOCOL_VERSION)" \
		-o "$(SURF_EXE)" ./cmd/surf

surf-dist: surf-binary
	rm -rf "dist/$(SURF_DIST)" "dist/Surf.app"
	if [ "$(SURF_GOOS)" = "darwin" ]; then \
		mkdir -p "dist/Surf.app/Contents/MacOS"; \
		cp "backend/$(SURF_EXE)" "dist/Surf.app/Contents/MacOS/surf"; \
		mkdir -p "dist/Surf.app/Contents/Resources"; \
		cp CHANGELOG.md "dist/Surf.app/Contents/Resources/CHANGELOG.md"; \
		(cd backend && go run ./tools/icns cmd/surf/surf-icon.png ../dist/Surf.app/Contents/Resources/Surf.icns); \
		sed "s/@VERSION@/$(VERSION)/g" packaging/desktop/Info.plist > "dist/Surf.app/Contents/Info.plist"; \
		tar -C dist -czf "dist/$(SURF_ARCHIVE)" "Surf.app"; \
	else \
		mkdir -p "dist/$(SURF_DIST)"; \
		cp "backend/$(SURF_EXE)" "dist/$(SURF_DIST)/$(SURF_EXE)"; \
		cp packaging/desktop/README.md "dist/$(SURF_DIST)/README.md"; \
		cp CHANGELOG.md "dist/$(SURF_DIST)/CHANGELOG.md"; \
		if [ "$(SURF_GOOS)" = "linux" ]; then cp packaging/backend/surf.service "dist/$(SURF_DIST)/surf.service"; fi; \
		if [ "$(SURF_GOOS)" = "windows" ]; then \
			rm -f "dist/$(SURF_ARCHIVE)"; \
			(cd backend && go run ./tools/zipdir "../dist/$(SURF_DIST)" "../dist/$(SURF_ARCHIVE)"); \
			if command -v "$(MAKENSIS)" >/dev/null 2>&1; then \
				sed -e "s|@VERSION@|$(VERSION)|g" -e "s|@SOURCE@|$(CURDIR)/dist/$(SURF_DIST)|g" \
					-e "s|@OUTPUT_DIR@|$(CURDIR)/dist|g" packaging/windows/surf.nsi.in > dist/surf.nsi; \
				"$(MAKENSIS)" -V2 dist/surf.nsi; \
				rm dist/surf.nsi; \
			fi; \
		else \
			tar -C dist -czf "dist/$(SURF_ARCHIVE)" "$(SURF_DIST)"; \
			if [ "$(SURF_GOARCH)" = "amd64" ] && [ -n "$${APPIMAGETOOL:-}" ]; then \
				rm -rf dist/Surf.AppDir; \
				mkdir -p dist/Surf.AppDir/usr/bin dist/Surf.AppDir/usr/share/doc/surf; \
				cp backend/surf dist/Surf.AppDir/usr/bin/surf; \
				cp CHANGELOG.md dist/Surf.AppDir/usr/share/doc/surf/CHANGELOG.md; \
				cp packaging/desktop/surf.desktop dist/Surf.AppDir/surf.desktop; \
				cp backend/cmd/surf/surf-icon.png dist/Surf.AppDir/surf.png; \
				ln -s usr/bin/surf dist/Surf.AppDir/AppRun; \
				ARCH=x86_64 "$$APPIMAGETOOL" --appimage-extract-and-run dist/Surf.AppDir "dist/surf-$(VERSION)-linux-amd64.AppImage"; \
			fi; \
		fi; \
	fi

surf-release-dist:
	test -n "$(CLIENT_DEB)"
	$(MAKE) surf-dist SURF_GOOS=linux SURF_GOARCH=amd64 CLIENT_DEB="$(CLIENT_DEB)"
	$(MAKE) surf-dist SURF_GOOS=linux SURF_GOARCH=arm64 CLIENT_DEB="$(CLIENT_DEB)"
	$(MAKE) surf-dist SURF_GOOS=windows SURF_GOARCH=amd64 CLIENT_DEB="$(CLIENT_DEB)"
	$(MAKE) surf-dist SURF_GOOS=darwin SURF_GOARCH=amd64 CLIENT_DEB="$(CLIENT_DEB)"
	$(MAKE) surf-dist SURF_GOOS=darwin SURF_GOARCH=arm64 CLIENT_DEB="$(CLIENT_DEB)"

native-sdk:
	native/buildenv/fetch-sdk.sh

native-package: native-sdk
	docker run --rm --network host -v "$(CURDIR):/src" surf-buildenv bash -c \
		'make -C /src/native/client clean package DEBUG=0 && bash /src/native/client/verify-package.sh'
