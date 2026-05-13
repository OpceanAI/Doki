use std::io::{BufRead, BufReader, Write};
use std::os::unix::net::UnixListener;
use std::path::Path;
use std::sync::mpsc;
use std::thread;

/// Command received from the host via vsock/unix socket.
#[derive(serde::Deserialize, Debug)]
#[serde(tag = "type")]
pub enum HostCommand {
    #[serde(rename = "exec")]
    Exec {
        #[serde(default)]
        id: String,
        cmd: Vec<String>,
        #[serde(default)]
        env: Vec<String>,
        #[serde(default)]
        cwd: Option<String>,
    },
    #[serde(rename = "signal")]
    Signal {
        sig: String,
    },
    #[serde(rename = "health")]
    Health,
    #[serde(rename = "shutdown")]
    Shutdown,
}

/// Response sent back to the host.
#[derive(serde::Serialize)]
#[serde(tag = "type")]
pub enum GuestResponse<'a> {
    #[serde(rename = "stdout")]
    Stdout { id: String, data: &'a str },
    #[serde(rename = "stderr")]
    Stderr { id: String, data: &'a str },
    #[serde(rename = "exit")]
    Exit { id: String, code: i32 },
    #[serde(rename = "health")]
    Health { status: &'a str, pid: i32 },
}

/// Channel type for incoming commands.
pub type CommandReceiver = mpsc::Receiver<HostCommand>;

/// Start the vsock listener in a background thread.
/// Returns a channel receiver for incoming commands.
pub fn start_server(socket_path: &str) -> CommandReceiver {
    let (tx, rx) = mpsc::channel::<HostCommand>();
    let path = socket_path.to_string();

    thread::spawn(move || {
        // Remove stale socket file
        let _ = std::fs::remove_file(&path);

        let listener = match UnixListener::bind(Path::new(&path)) {
            Ok(l) => {
                eprintln!("doki-init: listening on {}", path);
                l
            }
            Err(e) => {
                eprintln!("doki-init: failed to bind {}: {}", path, e);
                return;
            }
        };

        for stream in listener.incoming() {
            match stream {
                Ok(stream) => {
                    let tx = tx.clone();
                    thread::spawn(move || {
                        let mut reader = BufReader::new(&stream);
                        let mut line = String::new();

                        loop {
                            line.clear();
                            match reader.read_line(&mut line) {
                                Ok(0) => break, // EOF
                                Ok(_) => {
                                    let trimmed = line.trim();
                                    if trimmed.is_empty() {
                                        continue;
                                    }
                                    match serde_json::from_str::<HostCommand>(trimmed) {
                                        Ok(cmd) => {
                                            if tx.send(cmd).is_err() {
                                                break;
                                            }
                                        }
                                        Err(e) => {
                                            eprintln!("doki-init: bad JSON: {}", e);
                                        }
                                    }
                                }
                                Err(_) => break,
                            }
                        }
                    });
                }
                Err(e) => {
                    eprintln!("doki-init: accept error: {}", e);
                }
            }
        }
    });

    rx
}

/// Write a JSON response line to a writer.
pub fn write_response(w: &mut impl Write, resp: &GuestResponse) {
    if let Ok(json) = serde_json::to_string(resp) {
        let _ = write!(w, "{}\n", json);
        let _ = w.flush();
    }
}
