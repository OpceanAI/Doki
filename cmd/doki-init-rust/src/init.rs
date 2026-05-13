use std::ffi::CString;
use std::fs;
use std::io;
use std::os::unix::fs::symlink;

/// Mount essential virtual filesystems.
/// Only active when running as PID 1 (inside a microVM).
pub fn mount_essential_fs() {
    if unsafe { libc::getpid() } != 1 {
        return; // Not PID 1, skip (testing outside VM)
    }

    let mounts: &[(&str, &str, &str, libc::c_ulong)] = &[
        ("proc",     "/proc", "proc",     (libc::MS_NOSUID | libc::MS_NOEXEC | libc::MS_NODEV) as libc::c_ulong),
        ("sysfs",    "/sys",  "sysfs",    (libc::MS_NOSUID | libc::MS_NOEXEC | libc::MS_NODEV) as libc::c_ulong),
        ("devtmpfs", "/dev",  "devtmpfs", (libc::MS_NOSUID) as libc::c_ulong),
        ("tmpfs",    "/tmp",  "tmpfs",    0),
        ("tmpfs",    "/run",  "tmpfs",    0),
    ];

    for (source, target, fstype, flags) in mounts {
        let _ = fs::create_dir_all(target);

        let src_c = CString::new(*source).unwrap_or_default();
        let tgt_c = CString::new(*target).unwrap_or_default();
        let fst_c = CString::new(*fstype).unwrap_or_default();

        let ret = unsafe {
            libc::mount(
                src_c.as_ptr(),
                tgt_c.as_ptr(),
                fst_c.as_ptr(),
                *flags,
                std::ptr::null::<libc::c_void>(),
            )
        };

        if ret != 0 {
            let e = io::Error::last_os_error();
            let errno = e.raw_os_error().unwrap_or(0);
            if errno != libc::EBUSY {
                eprintln!("doki-init: mount {} on {} errno={}", fstype, target, errno);
            }
        }
    }
}

/// Create standard /dev symlinks.
pub fn create_dev_symlinks() {
    let links: &[(&str, &str)] = &[
        ("/proc/self/fd",   "/dev/fd"),
        ("/proc/self/fd/0", "/dev/stdin"),
        ("/proc/self/fd/1", "/dev/stdout"),
        ("/proc/self/fd/2", "/dev/stderr"),
    ];

    for (target, linkpath) in links {
        let _ = fs::remove_file(linkpath);
        if let Err(e) = symlink(target, linkpath) {
            if e.raw_os_error() != Some(libc::EEXIST) {
                eprintln!("doki-init: symlink {} -> {} failed: {}", linkpath, target, e);
            }
        }
    }
}
