.PHONY: all build build-android build-linux build-darwin test vet lint clean install \
        build-release sha256 version-info

GO = go
PREFIX ?= /data/data/com.termux/files/usr
RELEASES = releases

# Set GIT_TAG, GIT_COMMIT, BUILD_DATE, BUILD_USER at build time.
# Example:  make build-release GIT_TAG=v0.9.2

GIT_TAG    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
BUILD_USER ?= $(shell id -un 2>/dev/null || echo unknown)
DOKI_PKG   = github.com/OpceanAI/Doki/pkg/common
DOKI_API   = 1.48
DOKI_VER   = 0.9.3

# 16KB page size alignment (Android 15+ requirement, May 2026).
ifeq ($(GOOS),android)
    PAGE_SIZE_LDFLAGS = -Wl,-z,max-page-size=16384 -Wl,-z,common-page-size=16384
endif

LDFLAGS = -s -w \
    -X '$(DOKI_PKG).Version=$(DOKI_VER)' \
    -X '$(DOKI_PKG).DokiVersion=$(DOKI_VER)' \
    -X '$(DOKI_PKG).DokiAPIVersion=$(DOKI_API)' \
    -X '$(DOKI_PKG).GitCommit=$(GIT_COMMIT)' \
    -X '$(DOKI_PKG).BuildDate=$(BUILD_DATE)' \
    -X '$(DOKI_PKG).BuildUser=$(BUILD_USER)' \
    $(PAGE_SIZE_LDFLAGS)

GOFLAGS = -trimpath -ldflags="$(LDFLAGS)"

all: build

build: build-android build-linux build-darwin

$(RELEASES):
	mkdir -p $(RELEASES)


build-android: build-android-arm64 build-android-armv7

build-android-arm64: | $(RELEASES)
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-android-arm64 ./cmd/doki
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/dokid-android-arm64 ./cmd/dokid
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-compose-android-arm64 ./cmd/doki-compose
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-init-android-arm64 ./cmd/doki-init

build-android-armv7: | $(RELEASES)
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-android-armv7 ./cmd/doki
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/dokid-android-armv7 ./cmd/dokid
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-compose-android-armv7 ./cmd/doki-compose
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-init-android-armv7 ./cmd/doki-init


build-linux: build-linux-arm64 build-linux-armv7

build-linux-arm64: | $(RELEASES)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-linux-arm64 ./cmd/doki
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/dokid-linux-arm64 ./cmd/dokid
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-compose-linux-arm64 ./cmd/doki-compose
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-init-linux-arm64 ./cmd/doki-init

build-linux-armv7: | $(RELEASES)
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-linux-armv7 ./cmd/doki
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/dokid-linux-armv7 ./cmd/dokid
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-compose-linux-armv7 ./cmd/doki-compose
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-init-linux-armv7 ./cmd/doki-init


build-darwin: build-darwin-arm64

build-darwin-arm64: | $(RELEASES)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(RELEASES)/doki-darwin-arm64 ./cmd/doki


build-release: build-android build-linux build-darwin

sha256: | $(RELEASES)
	cd $(RELEASES) && for f in *; do \
		if [ -f "$$f" ] && [ "$${f##*.}" != "sha256" ]; then \
			sha256sum "$$f" > "$$f.sha256"; \
		fi; \
	done

release: build-release sha256
	@echo "\n=== Release v$(DOKI_VER) ready ==="
	@ls -lh $(RELEASES)/


version-info:
	@echo "Version:     $(DOKI_VER)"
	@echo "Git tag:     $(GIT_TAG)"
	@echo "Git commit:  $(GIT_COMMIT)"
	@echo "Build date:  $(BUILD_DATE)"
	@echo "Build user:  $(BUILD_USER)"
	@echo "API:         $(DOKI_API)"
	@echo "Platforms:   android-arm64 android-armv7 linux-arm64 linux-armv7 darwin-arm64"

test:
	$(GO) test ./... -count=1

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf $(RELEASES)/

install: build-android-arm64
	install -d $(PREFIX)/bin
	install $(RELEASES)/doki-android-arm64 $(PREFIX)/bin/doki
	install $(RELEASES)/dokid-android-arm64 $(PREFIX)/bin/dokid
	install $(RELEASES)/doki-compose-android-arm64 $(PREFIX)/bin/doki-compose
	install $(RELEASES)/doki-init-android-arm64 $(PREFIX)/bin/doki-init
