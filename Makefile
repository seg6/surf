.PHONY: test backend-binary backend-dist backend-dist-all native-sdk native-package

VERSION := $(shell tr -d '[:space:]' < VERSION)
PROTOCOL_VERSION := $(shell tr -d '[:space:]' < PROTOCOL_VERSION)
BACKEND_GOOS ?= $(shell go env GOOS)
BACKEND_GOARCH ?= $(shell go env GOARCH)
BACKEND_DIST := surf-backend-$(VERSION)-$(BACKEND_GOOS)-$(BACKEND_GOARCH)
ifeq ($(BACKEND_GOOS),windows)
BACKEND_EXE := surf-backend.exe
BACKEND_ARCHIVE := $(BACKEND_DIST).zip
else
BACKEND_EXE := surf-backend
BACKEND_ARCHIVE := $(BACKEND_DIST).tar.gz
endif

test:
	cd backend && go test ./...

backend-binary:
	cd backend && CGO_ENABLED=0 GOOS=$(BACKEND_GOOS) GOARCH=$(BACKEND_GOARCH) go build -trimpath \
		-ldflags="-s -w -X surf-backend/internal/config.AppVersion=$(VERSION) -X surf-backend/internal/config.NativeVersion=$(PROTOCOL_VERSION)" \
		-o "$(BACKEND_EXE)" ./cmd/surf-backend

backend-dist: backend-binary
	rm -rf "dist/$(BACKEND_DIST)"
	mkdir -p "dist/$(BACKEND_DIST)"
	cp "backend/$(BACKEND_EXE)" "dist/$(BACKEND_DIST)/$(BACKEND_EXE)"
	cp packaging/backend/README.md "dist/$(BACKEND_DIST)/README.md"
	if [ "$(BACKEND_GOOS)" = "linux" ]; then cp packaging/backend/surf-backend.service "dist/$(BACKEND_DIST)/surf-backend.service"; fi
	if [ "$(BACKEND_GOOS)" = "windows" ]; then \
		rm -f "dist/$(BACKEND_ARCHIVE)"; \
		cd backend && go run ./tools/zipdir "../dist/$(BACKEND_DIST)" "../dist/$(BACKEND_ARCHIVE)"; \
	else \
		tar -C dist -czf "dist/$(BACKEND_ARCHIVE)" "$(BACKEND_DIST)"; \
	fi

backend-dist-all:
	$(MAKE) backend-dist BACKEND_GOOS=linux BACKEND_GOARCH=amd64
	$(MAKE) backend-dist BACKEND_GOOS=windows BACKEND_GOARCH=amd64
	$(MAKE) backend-dist BACKEND_GOOS=darwin BACKEND_GOARCH=amd64
	$(MAKE) backend-dist BACKEND_GOOS=darwin BACKEND_GOARCH=arm64
	$(MAKE) backend-binary
	rm -f backend/surf-backend.exe

native-sdk:
	native/buildenv/fetch-sdk.sh

native-package: native-sdk
	docker run --rm --network host -v "$(CURDIR):/src" surf-buildenv make -C /src/native/client clean package DEBUG=0
