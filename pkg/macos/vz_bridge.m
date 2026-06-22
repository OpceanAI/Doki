// vz_bridge.m — Objective-C implementation of the VZ bridge.
//
// This file wraps the macOS Virtualization.framework (VZ) APIs into
// C-callable functions for use from Go via cgo.
//
// Build: requires macOS 11+, Xcode, CGO_ENABLED=1, and the
// com.apple.vm.hypervisor entitlement for HVF acceleration.

//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <Virtualization/Virtualization.h>
#import <objc/runtime.h>

#include "vz_bridge.h"

// Internal structures wrapping VZ objects.
struct vz_config {
    VZVirtualMachineConfiguration *config;
    VZNATNetworkDeviceAttachment *natAttachment;
    NSMutableArray<NSNumber *> *hostPorts;
    NSMutableArray<NSNumber *> *guestPorts;
}

struct vz_vm {
    VZVirtualMachine *vm;
    dispatch_queue_t queue;
    int state; // 0=stopped, 1=running, 2=paused, 3=starting
}

// Helper to create an NSError -> C string.
static char *error_to_string(NSError *err) {
    if (err == nil) return NULL;
    return strdup([[err localizedDescription] UTF8String]);
}

int vz_capabilities_available(void) {
    // Virtualization.framework requires macOS 11+.
    if (@available(macOS 11.0, *)) {
        // Check for hypervisor support.
        int hv_support = 0;
        size_t len = sizeof(hv_support);
        sysctlbyname("kern.hv_support", &hv_support, &len, NULL, 0);
        return hv_support ? 1 : 0;
    }
    return 0;
}

vz_config_t *vz_create_config(int cpu_count,
                              int64_t memory_bytes,
                              const char *disk_path,
                              int64_t disk_size_bytes,
                              const char *kernel_path,
                              const char *initrd_path,
                              const char *bootloader_type,
                              char **error_msg) {
    if (error_msg) *error_msg = NULL;

    @try {
        vz_config_t *cfg = (vz_config_t *)calloc(1, sizeof(vz_config_t));
        if (!cfg) {
            if (error_msg) *error_msg = strdup("out of memory");
            return NULL;
        }

        VZVirtualMachineConfiguration *vmConfig = [[VZVirtualMachineConfiguration alloc] init];

        // CPU count.
        vmConfig.cpuCount = cpu_count;
        if (vmConfig.cpuCount < 1) vmConfig.cpuCount = 1;

        // Memory.
        vmConfig.memorySize = (NSUInteger)memory_bytes;

        // Boot loader.
        NSString *btType = bootloader_type ? [NSString stringWithUTF8String:bootloader_type] : @"linux";
        if ([btType isEqualToString:@"macos"]) {
            VZMacOSBootLoader *bootLoader = [[VZMacOSBootLoader alloc] init];
            vmConfig.bootLoader = bootLoader;
        } else {
            // Linux boot loader.
            VZLinuxBootLoader *bootLoader = [[VZLinuxBootLoader alloc] init];
            if (kernel_path) {
                bootLoader.kernelPath = [NSString stringWithUTF8String:kernel_path];
            }
            if (initrd_path) {
                bootLoader.initrdPath = [NSString stringWithUTF8String:initrd_path_path];
            }
            // Default kernel command line.
            bootLoader.commandLine = @"console=ttyAMA0 root=/dev/vda rw";
            vmConfig.bootLoader = bootLoader;
        }

        // Block device (disk).
        if (disk_path) {
            NSString *diskPathStr = [NSString stringWithUTF8String:disk_path];
            NSURL *diskURL = [NSURL fileURLWithPath:diskPathStr];
            VZDiskImageStorageDeviceAttachment *diskAttachment =
                [[VZDiskImageStorageDeviceAttachment alloc] initWithURL:diskURL
                                                              readOnly:NO];
            VZVirtioBlockDeviceConfiguration *blockDevice =
                [[VZVirtioBlockDeviceConfiguration alloc] initWithAttachment:diskAttachment];
            vmConfig.storageDevices = @[blockDevice];
        }

        cfg->config = vmConfig;
        cfg->hostPorts = [[NSMutableArray alloc] init];
        cfg->guestPorts = [[NSMutableArray alloc] init];

        return cfg;
    } @catch (NSException *e) {
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return NULL;
    }
}

int vz_add_file_share(vz_config_t *cfg,
                      const char *host_path,
                      const char *guest_path,
                      const char *tag,
                      int read_only,
                      char **error_msg) {
    if (error_msg) *error_msg = NULL;
    if (!cfg || !cfg->config || !host_path) {
        if (error_msg) *error_msg = strdup("invalid arguments");
        return -1;
    }

    @try {
        NSString *hostPathStr = [NSString stringWithUTF8String:host_path];
        NSURL *hostURL = [NSURL fileURLWithPath:hostPathStr];
        VZSharedDirectory *sharedDir = [[VZSharedDirectory alloc] initWithURL:hostURL
                                                                   readOnly:(BOOL)read_only];
        NSString *tagStr = tag ? [NSString stringWithUTF8String:tag] : @"doki-share";
        VZVirtioFileSystemDeviceConfiguration *fsConfig =
            [[VZVirtioFileSystemDeviceConfiguration alloc] initWithTag:tagStr];
        fsConfig.directory = sharedDir;

        // Add to the configuration's directory sharing devices.
        NSMutableArray *devices = [cfg->config.directorySharingDevices mutableCopy];
        if (!devices) devices = [[NSMutableArray alloc] init];
        [devices addObject:fsConfig];
        cfg->config.directorySharingDevices = devices;

        return 0;
    } @catch (NSException *e) {
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return -1;
    }
}

int vz_add_nat_network(vz_config_t *cfg, char **error_msg) {
    if (error_msg) *error_msg = NULL;
    if (!cfg || !cfg->config) {
        if (error_msg) *error_msg = strdup("invalid arguments");
        return -1;
    }

    @try {
        VZNATNetworkDeviceAttachment *natAttachment = [[VZNATNetworkDeviceAttachment alloc] init];
        cfg->natAttachment = natAttachment;

        VZVirtioNetworkDeviceConfiguration *netConfig =
            [[VZVirtioNetworkDeviceConfiguration alloc] initWithAttachment:natAttachment];
        cfg->config.networkDevices = @[netConfig];

        return 0;
    } @catch (NSException *e) {
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return -1;
    }
}

int vz_add_port_forward(vz_config_t *cfg,
                        int host_port,
                        int guest_port,
                        const char *protocol,
                        char **error_msg) {
    if (error_msg) *error_msg = NULL;
    if (!cfg) {
        if (error_msg) *error_msg = strdup("invalid arguments");
        return -1;
    }

    @try {
        [cfg->hostPorts addObject:[NSNumber numberWithInt:host_port]];
        [cfg->guestPorts addObject:[NSNumber numberWithInt:guest_port]];
        // Port forwarding via NAT is handled by the OS when the NAT
        // attachment is created. Full port forwarding requires
        // VZVirtioNetworkDevice with a custom NAT configuration.
        // For now, we record the ports for reference.
        return 0;
    } @catch (NSException *e) {
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return -1;
    }
}

int vz_enable_rosetta(vz_config_t *cfg, char **error_msg) {
    if (error_msg) *error_msg = NULL;
    if (!cfg || !cfg->config) {
        if (error_msg) *error_msg = strdup("invalid arguments");
        return -1;
    }

    @try {
        if (@available(macOS 13.0, *)) {
            VZLinuxRosettaDirectoryShare *rosettaShare =
                [[VZLinuxRosettaDirectoryShare alloc] init];
            VZVirtioFileSystemDeviceConfiguration *rosettaConfig =
                [[VZVirtioFileSystemDeviceConfiguration alloc] initWithTag:@"rosetta"];
            rosettaConfig.directory = rosettaShare;

            NSMutableArray *devices = [cfg->config.directorySharingDevices mutableCopy];
            if (!devices) devices = [[NSMutableArray alloc] init];
            [devices addObject:rosettaConfig];
            cfg->config.directorySharingDevices = devices;
            return 0;
        } else {
            if (error_msg) *error_msg = strdup("Rosetta requires macOS 13+");
            return -1;
        }
    } @catch (NSException *e) {
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return -1;
    }
}

int vz_validate(vz_config_t *cfg, char **error_msg) {
    if (error_msg) *error_msg = NULL;
    if (!cfg || !cfg->config) {
        if (error_msg) *error_msg = strdup("invalid arguments");
        return -1;
    }

    @try {
        NSError *err = nil;
        if (![cfg->config validateWithError:&err]) {
            if (error_msg) *error_msg = error_to_string(err);
            return -1;
        }
        return 0;
    } @catch (NSException *e) {
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return -1;
    }
}

vz_vm_t *vz_create_vm(vz_config_t *cfg, char **error_msg) {
    if (error_msg) *error_msg = NULL;
    if (!cfg || !cfg->config) {
        if (error_msg) *error_msg = strdup("invalid arguments");
        return NULL;
    }

    @try {
        // Validate before creating.
        NSError *err = nil;
        if (![cfg->config validateWithError:&err]) {
            if (error_msg) *error_msg = error_to_string(err);
            return NULL;
        }

        dispatch_queue_t queue = dispatch_queue_create("com.doki.vz", DISPATCH_QUEUE_SERIAL);
        VZVirtualMachine *vm = [[VZVirtualMachine alloc] initWithConfiguration:cfg->config
                                                                          queue:queue];

        vz_vm_t *handle = (vz_vm_t *)calloc(1, sizeof(vz_vm_t));
        if (!handle) {
            if (error_msg) *error_msg = strdup("out of memory");
            return NULL;
        }
        handle->vm = vm;
        handle->queue = queue;
        handle->state = 0; // stopped

        return handle;
    } @catch (NSException *e) {
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return NULL;
    }
}

int vz_start_vm(vz_vm_t *vm, char **error_msg) {
    if (error_msg) *error_msg = NULL;
    if (!vm || !vm->vm) {
        if (error_msg) *error_msg = strdup("invalid VM handle");
        return -1;
    }

    @try {
        vm->state = 3; // starting

        NSError *err = nil;
        BOOL ok = [vm->vm startAndReturnError:&err];
        if (!ok) {
            vm->state = 0; // stopped
            if (error_msg) *error_msg = error_to_string(err);
            return -1;
        }
        vm->state = 1; // running
        return 0;
    } @catch (NSException *e) {
        vm->state = 0;
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return -1;
    }
}

int vz_stop_vm(vz_vm_t *vm, int timeout_sec, char **error_msg) {
    if (error_msg) *error_msg = NULL;
    if (!vm || !vm->vm) {
        if (error_msg) *error_msg = strdup("invalid VM handle");
        return -1;
    }

    @try {
        // Request graceful stop.
        [vm->vm stopWithCompletionHandler:^(NSError *err) {
            if (err) {
                // If graceful stop fails, force stop.
                [vm->vm stop];
            }
            vm->state = 0; // stopped
        }];

        // Wait for the stop to complete (simplified — a real
        // implementation would use a dispatch semaphore with the
        // timeout). For now, we return immediately; the state will
        // be updated by the completion handler.
        return 0;
    } @catch (NSException *e) {
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return -1;
    }
}

int vz_pause_vm(vz_vm_t *vm, char **error_msg) {
    if (error_msg) *error_msg = NULL;
    if (!vm || !vm->vm) {
        if (error_msg) *error_msg = strdup("invalid VM handle");
        return -1;
    }

    @try {
        NSError *err = nil;
        if (![vm->vm pauseAndReturnError:&err]) {
            if (error_msg) *error_msg = error_to_string(err);
            return -1;
        }
        vm->state = 2; // paused
        return 0;
    } @catch (NSException *e) {
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return -1;
    }
}

int vz_resume_vm(vz_vm_t *vm, char **error_msg) {
    if (error_msg) *error_msg = NULL;
    if (!vm || !vm->vm) {
        if (error_msg) *error_msg = strdup("invalid VM handle");
        return -1;
    }

    @try {
        NSError *err = nil;
        if (![vm->vm resumeAndReturnError:&err]) {
            if (error_msg) *error_msg = error_to_string(err);
            return -1;
        }
        vm->state = 1; // running
        return 0;
    } @catch (NSException *e) {
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return -1;
    }
}

int vz_vm_state(vz_vm_t *vm) {
    if (!vm) return -1;
    return vm->state;
}

int vz_vm_stats(vz_vm_t *vm, vz_stats_t *stats, char **error_msg) {
    if (error_msg) *error_msg = NULL;
    if (!vm || !vm->vm || !stats) {
        if (error_msg) *error_msg = strdup("invalid arguments");
        return -1;
    }

    @try {
        // VZ doesn't expose detailed stats directly; we report the
        // state and configured memory. A real implementation would
        // use VZVirtualMachine's observation APIs or Mach IPC.
        memset(stats, 0, sizeof(vz_stats_t));
        stats->memory_total = (int64_t)vm->vm.configuration.memorySize;
        stats->cpu_usage = 0.0; // would require sampling
        return 0;
    } @catch (NSException *e) {
        if (error_msg) *error_msg = strdup([[e reason] UTF8String]);
        return -1;
    }
}

void vz_delete_vm(vz_vm_t *vm) {
    if (!vm) return;
    // The VZVirtualMachine and dispatch_queue are managed by ARC.
    // We just free the C struct.
    free(vm);
}

void vz_delete_config(vz_config_t *cfg) {
    if (!cfg) return;
    // VZ objects are managed by ARC. We just free the C struct.
    free(cfg);
}

void vz_free_string(char *s) {
    if (s) free(s);
}
