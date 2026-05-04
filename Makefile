.PHONY: all build build-android build-linux build-darwin test lint clean install

BINARIES = doki dokid doki-compose
GO = go
GOFLAGS = -trimpath -ldflags="-s -w"
PREFIX ?= /data/data/com.termux/files/usr

all: build

build: build-android

build-android:
	GOOS=android GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/doki ./cmd/doki
	GOOS=android GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/dokid ./cmd/dokid
	GOOS=android GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/doki-compose ./cmd/doki-compose

build-linux:
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/linux/doki ./cmd/doki
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/linux/dokid ./cmd/dokid
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/linux/doki-compose ./cmd/doki-compose

build-linux-arm64:
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/linux-arm64/doki ./cmd/doki
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/linux-arm64/dokid ./cmd/dokid
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/linux-arm64/doki-compose ./cmd/doki-compose

build-darwin:
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/darwin/doki ./cmd/doki
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/darwin/dokid ./cmd/dokid
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o bin/darwin/doki-compose ./cmd/doki-compose

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/darwin-arm64/doki ./cmd/doki
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/darwin-arm64/dokid ./cmd/dokid
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o bin/darwin-arm64/doki-compose ./cmd/doki-compose

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
