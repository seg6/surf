.PHONY: test surf-binary surf-dist native-sdk native-package

VERSION := $(shell tr -d '[:space:]' < VERSION)
PROTOCOL_VERSION := $(shell tr -d '[:space:]' < PROTOCOL_VERSION)
SURF_GOOS ?= $(shell go env GOOS)
SURF_GOARCH ?= $(shell go env GOARCH)
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
	cd backend && CGO_ENABLED=1 GOOS=$(SURF_GOOS) GOARCH=$(SURF_GOARCH) go build -trimpath \
		-ldflags="-s -w -X surf-backend/internal/config.AppVersion=$(VERSION) -X surf-backend/internal/config.NativeVersion=$(PROTOCOL_VERSION)" \
		-o "$(SURF_EXE)" ./cmd/surf-desktop

surf-dist: surf-binary
	rm -rf "dist/$(SURF_DIST)" "dist/Surf.app"
	if [ "$(SURF_GOOS)" = "darwin" ]; then \
		mkdir -p "dist/Surf.app/Contents/MacOS"; \
		cp "backend/$(SURF_EXE)" "dist/Surf.app/Contents/MacOS/surf"; \
		sed "s/@VERSION@/$(VERSION)/g" packaging/desktop/Info.plist > "dist/Surf.app/Contents/Info.plist"; \
		tar -C dist -czf "dist/$(SURF_ARCHIVE)" "Surf.app"; \
	else \
		mkdir -p "dist/$(SURF_DIST)"; \
		cp "backend/$(SURF_EXE)" "dist/$(SURF_DIST)/$(SURF_EXE)"; \
		cp packaging/desktop/README.md "dist/$(SURF_DIST)/README.md"; \
		if [ "$(SURF_GOOS)" = "linux" ]; then cp packaging/backend/surf.service "dist/$(SURF_DIST)/surf.service"; fi; \
		if [ "$(SURF_GOOS)" = "windows" ]; then \
			rm -f "dist/$(SURF_ARCHIVE)"; \
			cd backend && go run ./tools/zipdir "../dist/$(SURF_DIST)" "../dist/$(SURF_ARCHIVE)"; \
		else \
			tar -C dist -czf "dist/$(SURF_ARCHIVE)" "$(SURF_DIST)"; \
		fi; \
	fi

native-sdk:
	native/buildenv/fetch-sdk.sh

native-package: native-sdk
	docker run --rm --network host -v "$(CURDIR):/src" surf-buildenv make -C /src/native/client clean package DEBUG=0
