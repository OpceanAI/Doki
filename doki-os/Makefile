# doki-OS Build System
# Builds: kernel + rootfs + VM image

DOKI_OS_VERSION ?= 0.10.0
ARCH ?= $(shell uname -m)
ifeq ($(ARCH),aarch64)
  ARCH = arm64
  QEMU_ARCH = aarch64
  KERNEL_IMAGE = arch/arm64/boot/Image.gz
endif
ifeq ($(ARCH),x86_64)
  ARCH = amd64
  QEMU_ARCH = x86_64
  KERNEL_IMAGE = arch/x86/boot/bzImage
endif

KERNEL_DIR = kernel
ROOTFS_DIR = rootfs
INIT_DIR = ../cmd/doki-init
OUTPUT_DIR = ../releases/doki-os

KERNEL_CONFIG = $(KERNEL_DIR)/doki-os.config
KERNEL_SRC ?= https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-6.12.tar.xz

ROOTFS_SIZE_MB ?= 50
VM_DISK_SIZE_GB ?= 20

.PHONY: all kernel rootfs image clean

all: image

# === Kernel ===
kernel: $(KERNEL_DIR)/linux/.config
	$(MAKE) -C $(KERNEL_DIR)/linux -j$$(nproc) $(KERNEL_IMAGE)
	cp $(KERNEL_DIR)/linux/$(KERNEL_IMAGE) $(OUTPUT_DIR)/vmlinuz

$(KERNEL_DIR)/linux/.config: $(KERNEL_CONFIG)
	@test -d $(KERNEL_DIR)/linux || (mkdir -p $(KERNEL_DIR) && cd $(KERNEL_DIR) && curl -sL $(KERNEL_SRC) | tar xJ && mv linux-* linux)
	cp $(KERNEL_CONFIG) $(KERNEL_DIR)/linux/.config
	$(MAKE) -C $(KERNEL_DIR)/linux olddefconfig

# === Rootfs ===
rootfs:
	@mkdir -p $(ROOTFS_DIR)/{bin,sbin,lib,etc,proc,sys,dev,tmp,run,var/lib/doki,host}
	@echo "Building rootfs for $(ARCH)..."
	@# Download Alpine mini rootfs
	@ALPINE_VER=3.20.3 && \
	curl -sL "https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/$(ARCH)/alpine-minirootfs-$$ALPINE_VER-$(ARCH).tar.gz" | \
	tar xz -C $(ROOTFS_DIR)
	@# Copy doki binaries
	@cp ../releases/doki-init $(ROOTFS_DIR)/init 2>/dev/null || true
	@cp ../releases/dokid $(ROOTFS_DIR)/bin/dokid-inner 2>/dev/null || true
	@cp ../releases/doki $(ROOTFS_DIR)/bin/doki 2>/dev/null || true
	@# Config
	@echo '{"storage_driver":"overlay2","default_network":"bridge","dns":{"listen":"127.0.0.11:53"}}' > $(ROOTFS_DIR)/etc/doki-config.json
	@# Inittab
	@echo '::sysinit:/init' > $(ROOTFS_DIR)/etc/inittab

# === VM Image ===
image: kernel rootfs
	@mkdir -p $(OUTPUT_DIR)
	@echo "Creating VM image..."
	@# Create ext4 rootfs image
	dd if=/dev/zero of=$(OUTPUT_DIR)/rootfs.ext4 bs=1M count=$(ROOTFS_SIZE_MB)
	mkfs.ext4 -F $(OUTPUT_DIR)/rootfs.ext4
	@# Mount and copy rootfs
	@mkdir -p /tmp/doki-os-mount
	@sudo mount -o loop $(OUTPUT_DIR)/rootfs.ext4 /tmp/doki-os-mount
	@sudo cp -a $(ROOTFS_DIR)/* /tmp/doki-os-mount/
	@sudo umount /tmp/doki-os-mount
	@# Create qcow2 disk
	qemu-img create -f qcow2 -b $(OUTPUT_DIR)/rootfs.ext4 -F raw \
		$(OUTPUT_DIR)/doki-os-$(ARCH).img $(VM_DISK_SIZE_GB)G
	@echo "VM image created: $(OUTPUT_DIR)/doki-os-$(ARCH).img"
	@# SHA256
	cd $(OUTPUT_DIR) && sha256sum doki-os-$(ARCH).img > doki-os-$(ARCH).img.sha256

clean:
	rm -rf $(KERNEL_DIR)/linux
	rm -rf $(ROOTFS_DIR)
	rm -rf $(OUTPUT_DIR)
