use std::io;
use std::os::unix::process::CommandExt;
use std::process::{Command, Stdio};

/// Execute a command and return its exit code.
/// If vsock_id is Some, stdout/stderr are piped and can be captured.
pub fn exec_command(
    args: &[String],
    envs: &[String],
    cwd: Option<&str>,
    vsock_id: &str,
) -> io::Result<(i32, Option<impl io::Read>, Option<impl io::Read>)> {
    if args.is_empty() {
        return Ok((1, None, None));
    }

    let mut cmd = Command::new(&args[0]);
    cmd.args(&args[1..]);

    // Set environment variables
    for e in envs {
        if let Some((key, val)) = e.split_once('=') {
            cmd.env(key, val);
        }
    }

    // Set working directory
    if let Some(dir) = cwd {
        cmd.current_dir(dir);
    }

    // If we have a vsock ID, pipe stdout/stderr so host can receive them
    if !vsock_id.is_empty() {
        cmd.stdout(Stdio::piped());
        cmd.stderr(Stdio::piped());
    }

    // Run in the same process group so signals propagate
    unsafe {
        cmd.pre_exec(|| {
            libc::setpgid(0, 0);
            Ok(())
        });
    }

    let mut child = cmd.spawn()?;

    let stdout = child.stdout.take();
    let stderr = child.stderr.take();

    let status = child.wait()?;

    let code = if status.success() {
        0
    } else {
        status.code().unwrap_or(127)
    };

    Ok((code, stdout, stderr))
}

/// Fork a process, run the command in the child, return the exit code via wait.
/// Used in default mode where init runs the command directly.
pub fn run_and_wait(args: &[String]) -> i32 {
    if args.is_empty() {
        return 1;
    }

    match Command::new(&args[0]).args(&args[1..]).spawn() {
        Ok(mut child) => match child.wait() {
            Ok(status) => {
                if status.success() {
                    0
                } else {
                    status.code().unwrap_or(1)
                }
            }
            Err(e) => {
                eprintln!("doki-init: wait failed: {}", e);
                1
            }
        },
        Err(e) => {
            eprintln!("doki-init: exec {} failed: {}", args[0], e);
            127
        }
    }
}
