// vz_bridge.h — Objective-C bridge to macOS Virtualization.framework.
//
// This header declares the C-callable functions that wrap the
// Virtualization.framework (VZ) APIs for use from Go via cgo.
//
// Build requirements:
//   - macOS 11.0+ (Big Sur) for Virtualization.framework
//   - Xcode with Objective-C compiler
//   - CGO_ENABLED=1
//   - Entitlement: com.apple.vm.hypervisor (for HVF acceleration)
//
// The bridge wraps:
//   - VZVirtualMachineConfiguration (CPU, memory, disk, kernel)
//   - VZLinuxBootLoader / VZMacOSBootLoader
//   - VZVirtioFileSystemDevice + VZSharedDirectory (directory sharing)
//   - VZBridgedNetworkDevice / VZNATNetworkDevice (networking)
//   - VZVirtioNetworkDevice (port forwarding via NAT)
//   - VZRosettaPlatform / VZLinuxRosettaDirectoryShare (Rosetta)
//   - VZVirtualMachineDelegate (state change callbacks)

#ifndef VZ_BRIDGE_H
#define VZ_BRIDGE_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// Opaque handle to a VZVirtualMachine.
typedef struct vz_vm vz_vm_t;

// Opaque handle to a VZVirtualMachineConfiguration.
typedef struct vz_config vz_config_t;

// vz_capabilities returns 1 if Virtualization.framework is available
// on this system, 0 otherwise.
int vz_capabilities_available(void);

// vz_create_config creates a new VM configuration.
// Returns NULL on error; error_msg is set to a human-readable string.
vz_config_t *vz_create_config(int cpu_count,
                              int64_t memory_bytes,
                              const char *disk_path,
                              int64_t disk_size_bytes,
                              const char *kernel_path,
                              const char *initrd_path,
                              const char *bootloader_type,
                              char **error_msg);

// vz_add_file_share adds a virtio-fs directory share to the config.
int vz_add_file_share(vz_config_t *cfg,
                      const char *host_path,
                      const char *guest_path,
                      const char *tag,
                      int read_only,
                      char **error_msg);

// vz_add_nat_network adds a NAT network device to the config.
// Returns the allocated host port base for forwarding.
int vz_add_nat_network(vz_config_t *cfg, char **error_msg);

// vz_add_port_forward adds a port forward rule to the NAT network.
int vz_add_port_forward(vz_config_t *cfg,
                        int host_port,
                        int guest_port,
                        const char *protocol,
                        char **error_msg);

// vz_enable_rosetta enables Rosetta 2 for x86 emulation on ARM.
int vz_enable_rosetta(vz_config_t *cfg, char **error_msg);

// vz_validate validates the configuration.
int vz_validate(vz_config_t *cfg, char **error_msg);

// vz_create_vm creates a VZVirtualMachine from the config.
vz_vm_t *vz_create_vm(vz_config_t *cfg, char **error_msg);

// vz_start_vm starts the virtual machine.
int vz_start_vm(vz_vm_t *vm, char **error_msg);

// vz_stop_vm stops the virtual machine with a timeout (seconds).
int vz_stop_vm(vz_vm_t *vm, int timeout_sec, char **error_msg);

// vz_pause_vm pauses the virtual machine.
int vz_pause_vm(vz_vm_t *vm, char **error_msg);

// vz_resume_vm resumes the paused virtual machine.
int vz_resume_vm(vz_vm_t *vm, char **error_msg);

// vz_vm_state returns the VM state:
//   0 = stopped, 1 = running, 2 = paused, 3 = starting, -1 = error
int vz_vm_state(vz_vm_t *vm);

// vz_vm_stats fills the stats struct with CPU/memory usage.
typedef struct vz_stats {
    double cpu_usage;
    int64_t memory_usage;
    int64_t memory_total;
    int64_t disk_read;
    int64_t disk_write;
    int64_t net_rx;
    int64_t net_tx;
} vz_stats_t;

int vz_vm_stats(vz_vm_t *vm, vz_stats_t *stats, char **error_msg);

// vz_delete_vm releases the VM and its resources.
void vz_delete_vm(vz_vm_t *vm);

// vz_delete_config releases the configuration.
void vz_delete_config(vz_config_t *cfg);

// vz_free_string frees a string allocated by the bridge.
void vz_free_string(char *s);

#ifdef __cplusplus
}
#endif

#endif // VZ_BRIDGE_H
