#!/bin/sh
set -e

VERSION="v0.9.3"
PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
BINDIR="${PREFIX}/bin"

if [ -n "${NO_COLOR:-}" ] || [ "${TERM:-}" = "dumb" ] || ! [ -t 1 ]; then
    BOLD="" RESET="" RED="" GREEN="" YELLOW="" CYAN=""
else
    BOLD='\033[1m' RESET='\033[0m'
    RED='\033[31m' GREEN='\033[32m'
    YELLOW='\033[33m' CYAN='\033[36m'
fi

step()  { printf "${CYAN}==>${RESET} %s\n" "$1"; }
ok()    { printf "${GREEN}OK${RESET} %s\n" "$1"; }
warn()  { printf "${YELLOW}WARNING:${RESET} %s\n" "$1" >&2; }
fail()  { printf "${RED}ERROR:${RESET} %s\n" "$1" >&2; exit 1; }

cleanup() {
    local rc=$?
    if [ -n "${WORK_DIR:-}" ] && [ -d "$WORK_DIR" ]; then
        rm -rf "$WORK_DIR"
    fi
    exit "$rc"
}
trap cleanup EXIT INT TERM

if [ "${1:-}" = "--uninstall" ]; then
    step "Uninstalling Doki ${VERSION}..."
    for bin in doki dokid doki-compose doki-init; do
        if [ -f "${BINDIR}/${bin}" ]; then
            rm -f "${BINDIR}/${bin}"
            printf "  removed %s/%s\n" "$BINDIR" "$bin"
        fi
    done
    ok "Doki uninstalled."
    exit 0
fi

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat << EOF
Doki ${VERSION} Installer

Usage: ./install.sh [OPTIONS]

Options:
  --help, -h       Show this help
  --uninstall      Remove Doki binaries

Environment:
  PREFIX           Install prefix (default: /data/data/com.termux/files/usr)
  NO_COLOR         Disable colored output
EOF
    exit 0
fi

printf "\n${BOLD}Doki ${VERSION} Installer${RESET}\n\n"

step "Detecting platform..."
ARCH=$(uname -m)
OS=$(uname -s)
case "$ARCH" in
    aarch64|arm64) ARCH_LABEL="android-arm64" ;;
    armv7*|armv8l)  ARCH_LABEL="linux-armv7" ;;
    x86_64|amd64)  ARCH_LABEL="linux-arm64" ;;
    *) fail "Unsupported architecture: $ARCH" ;;
esac
ok "Platform: ${OS} ${ARCH_LABEL}"

step "Checking installation directory..."
if [ ! -d "$BINDIR" ]; then
    install -d "$BINDIR" || fail "Cannot create ${BINDIR}"
fi
ok "Directory: ${BINDIR}"

step "Installing binaries..."
INSTALLED=0
for bin in doki dokid doki-compose doki-init; do
    if [ -f "./${bin}" ]; then
        install -m 0755 "./${bin}" "${BINDIR}/${bin}"
        printf "  ${GREEN}installed${RESET} %s\n" "$bin"
        INSTALLED=$((INSTALLED + 1))
    fi
done

if [ "$INSTALLED" -eq 0 ]; then
    fail "No binaries found. Archive may be corrupted."
fi
ok "${INSTALLED} binaries installed"

step "Verifying..."
if command -v doki >/dev/null 2>&1; then
    VER=$(doki version 2>/dev/null | head -1 || echo "unknown")
    ok "doki version: ${VER}"
else
    warn "doki not in PATH yet. Run: source ~/.bashrc or restart shell"
fi

printf "\n${GREEN}${BOLD}Done!${RESET}\n\n"
printf "Quick start:\n"
printf "  dokid &              Start daemon\n"
printf "  doki pull alpine     Pull base image\n"
printf "  doki run alpine sh   Run a shell\n"
printf "  doki ps              List containers\n"
printf "  doki mesh status     Show mesh identity\n"
printf "\n"
