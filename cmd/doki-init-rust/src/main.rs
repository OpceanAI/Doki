mod cmdline;
mod command;
mod init;
mod signal;
mod vsock;

use std::env;
use std::io::{BufReader, Read};
use std::os::unix::net::UnixStream;
use std::process;
use std::sync::mpsc;

use vsock::{CommandReceiver, GuestResponse, HostCommand};

fn main() {
    std::panic::set_hook(Box::new(|info| {
        eprintln!("doki-init panic: {}", info);
        process::abort();
    }));

    // Mount essential filesystems (no-op if not PID 1)
    init::mount_essential_fs();

    // Create /dev symlinks
    init::create_dev_symlinks();

    // Set up signal handlers
    signal::setup_handlers();

    // Parse command from kernel cmdline or argv
    let args = match cmdline::parse_doki_cmd() {
        Some(cmd) => cmd,
        None => {
            let argv: Vec<String> = env::args().collect();
            cmdline::args_from_argv(&argv)
        }
    };

    let _mode = env::var("DOKI_INIT_MODE").unwrap_or_default();

    // 5. Determine mode
    let mode = env::var("DOKI_INIT_MODE").unwrap_or_default();

    match mode.as_str() {
        "oneshot" => {
            let code = command::run_and_wait(&args);
            process::exit(code);
        }
        "vsock" => {
            let socket = env::var("DOKI_VSOCK_SOCKET")
                .unwrap_or_else(|_| "/tmp/doki-vsock.sock".to_string());
            let rx = vsock::start_server(&socket);
            run_vsock_loop(rx, &args);
        }
        _ => {
            // Default: run command in foreground, reaping children
            run_default(&args);
        }
    }
}

/// Default mode: run the command directly as init.
fn run_default(args: &[String]) {
    let code = command::run_and_wait(args);
    process::exit(code);
}

/// Vsock mode: listen for commands and execute them on demand.
fn run_vsock_loop(rx: CommandReceiver, _args: &[String]) {
    let mut child_pid: Option<i32> = None;

    while signal::is_running() {
        signal::reap_zombies();

        match rx.recv_timeout(std::time::Duration::from_millis(500)) {
            Ok(cmd) => {
                match cmd {
                    HostCommand::Exec {
                        id,
                        cmd: exec_args,
                        env: exec_env,
                        cwd,
                    } => {
                        let cwd_ref = cwd.as_deref();

                        match command::exec_command(&exec_args, &exec_env, cwd_ref, &id) {
                            Ok((code, stdout, stderr)) => {
                                child_pid = None;

                                // Stream stdout back to host
                                if let (Some(out), Some(conn_path)) = (
                                    stdout,
                                    env::var("DOKI_VSOCK_SOCKET").ok(),
                                ) {
                                    if let Ok(mut conn) =
                                        UnixStream::connect(&conn_path)
                                    {
                                        let id_copy = id.clone();
                                        stream_output(
                                            out,
                                            &mut conn,
                                            &id_copy,
                                            "stdout",
                                        );
                                    }
                                }

                                // Stream stderr back to host
                                if let (Some(err), Some(conn_path)) = (
                                    stderr,
                                    env::var("DOKI_VSOCK_SOCKET").ok(),
                                ) {
                                    if let Ok(mut conn) =
                                        UnixStream::connect(&conn_path)
                                    {
                                        let id_copy = id.clone();
                                        stream_output(
                                            err,
                                            &mut conn,
                                            &id_copy,
                                            "stderr",
                                        );
                                    }
                                }

                                // Send exit code
                                if let Ok(mut conn) =
                                    UnixStream::connect(
                                        &env::var("DOKI_VSOCK_SOCKET")
                                            .unwrap_or_default(),
                                    )
                                {
                                    let resp = GuestResponse::Exit {
                                        id: id.clone(),
                                        code,
                                    };
                                    vsock::write_response(&mut conn, &resp);
                                }
                            }
                            Err(e) => {
                                eprintln!("doki-init: exec failed: {}", e);
                            }
                        }
                    }

                    HostCommand::Signal { sig } => {
                        eprintln!("doki-init: received signal request: {}", sig);
                        // Forward to child if running
                        if let Some(pid) = child_pid {
                            let signum = match sig.as_str() {
                                "SIGTERM" => libc::SIGTERM,
                                "SIGKILL" => libc::SIGKILL,
                                "SIGINT" => libc::SIGINT,
                                "SIGHUP" => libc::SIGHUP,
                                _ => libc::SIGTERM,
                            };
                            signal::send_signal(pid, signum);
                        }
                    }

                    HostCommand::Health => {
                        if let Ok(mut conn) = UnixStream::connect(
                            &env::var("DOKI_VSOCK_SOCKET").unwrap_or_default(),
                        ) {
                            let resp = GuestResponse::Health {
                                status: "ok",
                                pid: process::id() as i32,
                            };
                            vsock::write_response(&mut conn, &resp);
                        }
                    }

                    HostCommand::Shutdown => {
                        signal::kill_all_children();
                        process::exit(0);
                    }
                }
            }
            Err(mpsc::RecvTimeoutError::Timeout) => {
                // Normal: no message, keep looping
            }
            Err(mpsc::RecvTimeoutError::Disconnected) => {
                eprintln!("doki-init: vsock channel disconnected");
                break;
            }
        }
    }

    // Shutdown cleanup
    signal::kill_all_children();
}

/// Stream output from a child process to a socket as JSON lines.
fn stream_output<R: Read>(reader: R, writer: &mut UnixStream, id: &str, stream_type: &str) {
    let mut buf_reader = BufReader::new(reader);
    let mut buf = [0u8; 4096];

    loop {
        match buf_reader.read(&mut buf) {
            Ok(0) => break,
            Ok(n) => {
                let data = std::str::from_utf8(&buf[..n]).unwrap_or("");
                let resp = match stream_type {
                    "stdout" => GuestResponse::Stdout {
                        id: id.to_string(),
                        data,
                    },
                    "stderr" => GuestResponse::Stderr {
                        id: id.to_string(),
                        data,
                    },
                    _ => continue,
                };
                vsock::write_response(writer, &resp);
            }
            Err(_) => break,
        }
    }
}
