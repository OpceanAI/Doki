FROM golang:1.26.3-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS TARGETARCH
ARG GIT_TAG=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
ARG BUILD_USER=docker

ENV DOKI_PKG=github.com/OpceanAI/Doki/pkg/common \
    DOKI_API=1.54 \
    DOKI_VER=0.10.0 \
    GOFLAGS="-trimpath -ldflags=-s -w -X '${DOKI_PKG}.Version=${DOKI_VER}' \
        -X '${DOKI_PKG}.DokiVersion=${DOKI_VER}' \
        -X '${DOKI_PKG}.DokiAPIVersion=${DOKI_API}' \
        -X '${DOKI_PKG}.GitCommit=${GIT_COMMIT}' \
        -X '${DOKI_PKG}.BuildDate=${BUILD_DATE}' \
        -X '${DOKI_PKG}.BuildUser=${BUILD_USER}'"

RUN GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} CGO_ENABLED=0 go build -o /build/doki         ./cmd/doki && \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} CGO_ENABLED=0 go build -o /build/dokid        ./cmd/dokid && \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} CGO_ENABLED=0 go build -o /build/doki-compose ./cmd/doki-compose && \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} CGO_ENABLED=0 go build -o /build/doki-init    ./cmd/doki-init && \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} CGO_ENABLED=0 go build -o /build/doki-kube    ./cmd/doki-kube && \
    GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} CGO_ENABLED=0 go build -o /build/doki-kubectl ./cmd/doki-kubectl

FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /build/doki         /usr/bin/doki
COPY --from=builder /build/dokid        /usr/bin/dokid
COPY --from=builder /build/doki-compose /usr/bin/doki-compose
COPY --from=builder /build/doki-init    /usr/bin/doki-init
COPY --from=builder /build/doki-kube    /usr/bin/doki-kube
COPY --from=builder /build/doki-kubectl /usr/bin/doki-kubectl

RUN mkdir -p /etc/doki /var/lib/doki /var/log/doki

EXPOSE 8080 8443

ENTRYPOINT ["/usr/bin/doki"]
CMD ["--help"]
