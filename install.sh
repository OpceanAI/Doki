#!/bin/sh
set -e

PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
BINDIR="${PREFIX}/bin"

echo "Doki v0.9.3 installer"
echo "Installing to ${BINDIR}"

install -d "${BINDIR}"

for bin in doki dokid doki-compose doki-init; do
    if [ -f "./${bin}" ]; then
        install -m 0755 "./${bin}" "${BINDIR}/${bin}"
        echo "  installed ${bin}"
    fi
done

echo ""
echo "Done. Run 'dokid &' to start the daemon."
