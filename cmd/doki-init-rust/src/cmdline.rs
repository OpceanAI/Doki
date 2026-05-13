use std::fs;

/// Parse the command from /proc/cmdline.
/// Looks for "doki.cmd=" prefix, value is colon-separated args.
/// Example: "doki.cmd=/bin/sh:-c:echo hello" -> ["/bin/sh", "-c", "echo hello"]
pub fn parse_doki_cmd() -> Option<Vec<String>> {
    let content = fs::read_to_string("/proc/cmdline").ok()?;

    for word in content.split_whitespace() {
        if let Some(cmd_str) = word.strip_prefix("doki.cmd=") {
            let args = cmd_str.split(':').map(String::from).collect::<Vec<_>>();
            if !args.is_empty() {
                return Some(args);
            }
        }
    }
    None
}

/// Fallback: use program arguments.
pub fn args_from_argv(argv: &[String]) -> Vec<String> {
    if argv.len() > 1 {
        argv[1..].to_vec()
    } else {
        vec!["/bin/sh".to_string()]
    }
}
