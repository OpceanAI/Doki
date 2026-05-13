use std::sync::atomic::{AtomicBool, Ordering};

static RUNNING: AtomicBool = AtomicBool::new(true);

/// Install signal handlers for PID 1.
pub fn setup_handlers() {
    unsafe {
        // SIGCHLD: handled in the main event loop via waitpid
        libc::signal(libc::SIGCHLD, sigchld_handler as *const () as libc::sighandler_t);
        // SIGTERM / SIGINT: shutdown gracefully
        libc::signal(libc::SIGTERM, sigterm_handler as *const () as libc::sighandler_t);
        libc::signal(libc::SIGINT, sigterm_handler as *const () as libc::sighandler_t);
        // SIGHUP: reload config (no-op for now)
        libc::signal(libc::SIGHUP, libc::SIG_IGN);
    }
}

extern "C" fn sigchld_handler(_sig: libc::c_int) {
    reap_zombies();
}

extern "C" fn sigterm_handler(_sig: libc::c_int) {
    RUNNING.store(false, Ordering::SeqCst);
}

/// Reap all zombie child processes.
pub fn reap_zombies() {
    loop {
        let mut status: libc::c_int = 0;
        let pid = unsafe { libc::waitpid(-1, &mut status, libc::WNOHANG) };
        if pid <= 0 {
            break;
        }
    }
}

/// Check if the init process is still running.
pub fn is_running() -> bool {
    RUNNING.load(Ordering::SeqCst)
}

/// Send a signal to a child process by pid.
pub fn send_signal(pid: libc::c_int, sig: libc::c_int) -> bool {
    unsafe { libc::kill(pid, sig) == 0 }
}

/// Send SIGTERM to all child processes, wait, then SIGKILL.
pub fn kill_all_children() {
    // Send SIGTERM to all processes in the pid namespace
    unsafe { libc::kill(-1, libc::SIGTERM) };
    // Wait up to 5 seconds
    let mut waited = 0;
    while waited < 50 && has_children() {
        std::thread::sleep(std::time::Duration::from_millis(100));
        reap_zombies();
        waited += 1;
    }
    // Force kill remaining
    unsafe { libc::kill(-1, libc::SIGKILL) };
    reap_zombies();
}

fn has_children() -> bool {
    let mut status: libc::c_int = 0;
    let pid = unsafe { libc::waitpid(-1, &mut status, libc::WNOHANG) };
    pid > 0 || unsafe { libc::waitpid(0, &mut status, libc::WNOHANG) } > 0
}
