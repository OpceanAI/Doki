# FreeBSD Compatibility

Doki runs on FreeBSD in **ModeNative** only. The Linux-specific execution
modes (namespaces, cgroups, proot) are stubbed out and report unavailable.

## Status

| Feature | Status |
|:--------|:-------|
| ModeNative (direct exec) | **Tested** |
| Storage (nullfs RO + copy RW) | **Tested** |
| jail | **Untested** |
| vnet (network virtualization) | **Untested** |
| rctl (resource limits) | **Untested** |

## Implementation

- `internal/namespaces/stub_freebsd.go` — no-op namespace manager. FreeBSD
  has no unshare/clone flags, pivot_root, or persistent user namespaces.
- `internal/cgroups/stub_freebsd.go` — no-op cgroup manager. FreeBSD enforces
  resource limits with `rctl(8)` and jails, not cgroup v2.
- `pkg/storage/mount_freebsd.go` — mounts read-only layers with
  `mount -t nullfs` (bind-mount semantics); read-write layers are copied in
  ModeNative because nullfs provides no copy-on-write.

## Cross-build

CI cross-builds for FreeBSD without touching the release workflows:

```sh
GOOS=freebsd GOARCH=amd64 go build ./pkg/storage/... ./internal/namespaces/... ./internal/cgroups/...
```
