#!/bin/sh
echo "=== Doki Multi-Stage App ==="
echo "Version: ${APP_VERSION:-unknown}"
echo "Env: ${APP_ENV:-production}"
echo "Date: $(date 2>/dev/null || echo N/A)"
if [ -f /opt/app/config.txt ]; then
    echo "Config:"
    cat /opt/app/config.txt
fi
echo "============================="
