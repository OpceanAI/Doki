.PHONY: all build build-android build-linux build-darwin test lint clean install \
        build-init-rust build-init-rust-armv7

BINARIES = doki dokid doki-compose doki-init
GO = go
GOFLAGS = -trimpath -ldflags="-s -w"
PREFIX ?= /data/data/com.termux/files/usr
CARGO = cargo

all: build

build: build-android build-init-rust

build-android:
	GOOS=android GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/doki ./cmd/doki
	GOOS=android GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/dokid ./cmd/dokid
	GOOS=android GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/doki-compose ./cmd/doki-compose
	GOOS=android GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/doki-init ./cmd/doki-init

build-linux:
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/linux/doki ./cmd/doki
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/linux/dokid ./cmd/dokid
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/linux/doki-compose ./cmd/doki-compose
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/linux/doki-init ./cmd/doki-init

build-linux-arm64:
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/linux-arm64/doki ./cmd/doki
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/linux-arm64/dokid ./cmd/dokid
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/linux-arm64/doki-compose ./cmd/doki-compose
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/linux-arm64/doki-init ./cmd/doki-init

build-darwin:
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/darwin/doki ./cmd/doki
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/darwin/dokid ./cmd/dokid
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/darwin/doki-compose ./cmd/doki-compose
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/darwin/doki-init ./cmd/doki-init

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/darwin-arm64/doki ./cmd/doki
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/darwin-arm64/dokid ./cmd/dokid
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/darwin-arm64/doki-compose ./cmd/doki-compose
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/darwin-arm64/doki-init ./cmd/doki-init

build-windows:
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/windows/doki.exe ./cmd/doki
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/windows/dokid.exe ./cmd/dokid
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/windows/doki-compose.exe ./cmd/doki-compose
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/windows/doki-init.exe ./cmd/doki-init

build-all: build-android build-linux build-linux-arm64 build-darwin build-darwin-arm64 build-windows

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

install: build-android
	install -d $(PREFIX)/bin
	install bin/doki $(PREFIX)/bin/doki
	install bin/dokid $(PREFIX)/bin/dokid
	install bin/doki-compose $(PREFIX)/bin/doki-compose

# ─── doki-init-rust (Rust) ──────────────────────────────────

build-init-rust:
	cd cmd/doki-init-rust && $(CARGO) build --release --target aarch64-linux-android
	cp cmd/doki-init-rust/target/aarch64-linux-android/release/doki-init-rust releases/doki-init-rust-android-arm64

build-init-rust-armv7:
	cd cmd/doki-init-rust && $(CARGO) build --release --target armv7-linux-androideabi
	cp cmd/doki-init-rust/target/armv7-linux-androideabi/release/doki-init-rust releases/doki-init-rust-android-armv7
