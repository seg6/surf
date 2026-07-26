.PHONY: test backend-binary backend-dist native-sdk native-package

VERSION := $(shell tr -d '[:space:]' < VERSION)
PROTOCOL_VERSION := $(shell tr -d '[:space:]' < PROTOCOL_VERSION)
BACKEND_GOOS ?= $(shell go env GOOS)
BACKEND_GOARCH ?= $(shell go env GOARCH)
BACKEND_DIST := surf-backend-$(VERSION)-$(BACKEND_GOOS)-$(BACKEND_GOARCH)

test:
	cd backend && go test ./...

backend-binary:
	cd backend && CGO_ENABLED=0 GOOS=$(BACKEND_GOOS) GOARCH=$(BACKEND_GOARCH) go build -trimpath \
		-ldflags="-s -w -X surf-backend/internal/config.AppVersion=$(VERSION) -X surf-backend/internal/config.NativeVersion=$(PROTOCOL_VERSION)" \
		-o surf-backend ./cmd/surf-backend

backend-dist: backend-binary
	rm -rf "dist/$(BACKEND_DIST)"
	mkdir -p "dist/$(BACKEND_DIST)"
	cp backend/surf-backend "dist/$(BACKEND_DIST)/surf-backend"
	cp packaging/backend/README.md "dist/$(BACKEND_DIST)/README.md"
	cp packaging/backend/surf-backend.service "dist/$(BACKEND_DIST)/surf-backend.service"
	tar -C dist -czf "dist/$(BACKEND_DIST).tar.gz" "$(BACKEND_DIST)"

native-sdk:
	native/buildenv/fetch-sdk.sh

native-package: native-sdk
	docker run --rm --network host -v "$(CURDIR):/src" surf-buildenv make -C /src/native/client clean package DEBUG=0
