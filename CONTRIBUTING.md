# Contributing to Doki

Thank you for your interest in contributing to Doki!

## Build Instructions

### Prerequisites

- Go 1.22 or later
- `make` (optional, for convenience targets)
- For Termux/Android: `pkg install golang make`

### Building

```bash
# Build all binaries for current platform
make build

# Build for specific platforms
make build-android        # GOOS=android GOARCH=arm64
make build-linux          # GOOS=linux GOARCH=amd64
make build-linux-arm64    # GOOS=linux GOARCH=arm64
make build-darwin         # GOOS=darwin GOARCH=amd64
make build-darwin-arm64   # GOOS=darwin GOARCH=arm64
make build-windows        # GOOS=windows GOARCH=amd64
make build-all            # Build all platforms

# Manual build
go build -trimpath -ldflags="-s -w" -o bin/doki ./cmd/doki
go build -trimpath -ldflags="-s -w" -o bin/dokid ./cmd/dokid
go build -trimpath -ldflags="-s -w" -o bin/doki-compose ./cmd/doki-compose
go build -trimpath -ldflags="-s -w" -o bin/doki-init ./cmd/doki-init
```

### Testing

```bash
# Run all tests
make test
# or
go test ./...

# Run tests for a specific package
go test ./pkg/builder/

# Run tests with race detection
go test -race ./...

# Run linter
make lint
# or
golangci-lint run ./...

# Run vet
make vet
# or
go vet ./...
```

## Code Style

- Follow standard Go conventions and idioms
- Run `gofmt` (or `gofumpt`) before committing
- Use `golangci-lint` with the project's `.golangci.yml` configuration
- Keep functions focused and single-purpose
- Use meaningful variable names
- Add tests for new functionality
- Write clear commit messages

## Project Structure

```
cmd/          - Binary entry points (doki, dokid, doki-compose, doki-init)
pkg/          - Public library packages
  api/        - Docker Engine API v1.44 server
  builder/    - Dockerfile/Dokifile parser and builder
  cli/        - CLI command implementation
  common/     - Shared types, configuration, utilities
  compose/    - Docker Compose engine
  image/      - OCI image store
  network/    - Container networking
  registry/   - OCI Distribution Spec client
  runtime/    - OCI runtime with 4 execution modes
  storage/    - Storage drivers
  cri/        - Kubernetes CRI plugin
internal/     - Internal subsystems
  dokivm/     - MicroVM subsystem
  namespaces/ - Linux namespace management
  cgroups/    - cgroups v2 resource management
  proot/      - proot fallback for Android
  fuse/       - FUSE overlay filesystem operations
  seccomp/    - Seccomp profile engine
  apparmor/   - AppArmor profile generator
```

## Pull Request Process

1. Fork the repository and create your branch from `main`
2. If you've added code, add tests covering the new functionality
3. Ensure all tests pass (`go test ./...`)
4. Run `go vet ./...` and `golangci-lint run ./...` to check for issues
5. Update documentation if needed
6. Follow the existing commit message style
7. Open a pull request with a clear description of the changes

## Areas for Contribution

- **MicroVM backends**: Support for additional hypervisors and platforms
- **CNI plugins**: Implementation of advanced networking features
- **Security**: Hardening, fuzzing, and penetration testing
- **Performance**: Layer caching, parallel operations, memory optimization
- **Testing**: Integration tests, end-to-end tests, stress tests
- **Documentation**: Tutorials, examples, and API reference

## Commit Style

- Use imperative mood ("Add feature" not "Added feature")
- Keep the first line under 72 characters
- Reference issues when applicable
- Example: `Add --log-level flag for daemon log verbosity control`
