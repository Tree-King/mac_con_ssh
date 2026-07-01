APP_NAME := ssh-tunnel
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DIST_DIR ?= dist
GO ?= go

.PHONY: build release clean test vet

build:
	GOTOOLCHAIN=local $(GO) build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(APP_NAME) ./cmd/ssh-tunnel

release:
	VERSION=$(VERSION) DIST_DIR=$(DIST_DIR) ./scripts/release.sh

test:
	GOTOOLCHAIN=local $(GO) test ./...

vet:
	GOTOOLCHAIN=local $(GO) vet ./...

clean:
	rm -rf $(DIST_DIR) $(APP_NAME)
