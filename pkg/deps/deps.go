// Package deps ensures all required dependencies are included in go.mod.
// This file imports all dependencies that Doki v0.10 requires.
package deps

import (
	// OCI specifications
	_ "github.com/opencontainers/image-spec/specs-go/v1"
	_ "github.com/opencontainers/runtime-spec/specs-go"
	_ "github.com/opencontainers/go-digest"
	_ "github.com/opencontainers/selinux/go-selinux"

	// gRPC and Protobuf (for CRI)
	_ "google.golang.org/grpc"
	_ "google.golang.org/protobuf/proto"

	// Kubernetes CRI API
	_ "k8s.io/cri-api/pkg/apis/runtime/v1"

	// Container runtime
	_ "github.com/containerd/containerd/v2/pkg/oci"

	// Compression
	_ "github.com/klauspost/compress/zstd"
	_ "github.com/ulikunitz/xz"

	// Docker/Moby utilities
	_ "github.com/moby/patternmatcher"
	_ "github.com/moby/term"

	// Database (SQLite for K8s state store)
	_ "github.com/ncruces/go-sqlite3"

	// Terminal utilities
	_ "github.com/mattn/go-isatty"
	_ "golang.org/x/term"
)
