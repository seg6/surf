.PHONY: test backend-docker native-sdk native-package

VERSION := $(shell tr -d '[:space:]' < VERSION)
PROTOCOL_VERSION := $(shell tr -d '[:space:]' < PROTOCOL_VERSION)

test:
	cd backend && go test ./...

backend-docker:
	docker build --network host \
		--build-arg VERSION="$(VERSION)" \
		--build-arg PROTOCOL_VERSION="$(PROTOCOL_VERSION)" \
		-t surf-backend:test backend

native-sdk:
	native/buildenv/fetch-sdk.sh

native-package: native-sdk
	docker run --rm --network host -v "$(CURDIR):/src" surf-buildenv make -C /src/native/client clean package DEBUG=0
