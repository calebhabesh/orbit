#!/usr/bin/env bash
# File Sync uninstallation script
set -euo pipefail

echo "Stopping and disabling filesync systemd user service..."
systemctl --user stop filesync.service 2>/dev/null || true
systemctl --user disable filesync.service 2>/dev/null || true

# Remove user-local installation if present
USER_BIN="${HOME}/.local/bin/filesync"
USER_SERVICE="${HOME}/.config/systemd/user/filesync.service"

if [ -f "${USER_BIN}" ]; then
    rm -f "${USER_BIN}"
    echo "Removed ${USER_BIN}"
fi

if [ -f "${USER_SERVICE}" ]; then
    rm -f "${USER_SERVICE}"
    echo "Removed ${USER_SERVICE}"
fi

# Remove system-wide installation if present and running with permissions
SYS_BIN="/usr/local/bin/filesync"
SYS_SERVICE="/usr/lib/systemd/user/filesync.service"

if [ -f "${SYS_BIN}" ] && [ -w "/usr/local/bin" ]; then
    rm -f "${SYS_BIN}"
    echo "Removed ${SYS_BIN}"
elif [ -f "${SYS_BIN}" ]; then
    echo "Notice: ${SYS_BIN} requires root permissions to remove (sudo rm -f ${SYS_BIN})"
fi

if [ -f "${SYS_SERVICE}" ] && [ -w "/usr/lib/systemd/user" ]; then
    rm -f "${SYS_SERVICE}"
    echo "Removed ${SYS_SERVICE}"
elif [ -f "${SYS_SERVICE}" ]; then
    echo "Notice: ${SYS_SERVICE} requires root permissions to remove (sudo rm -f ${SYS_SERVICE})"
fi

systemctl --user daemon-reload 2>/dev/null || true

echo ""
echo "Uninstallation complete."
echo "DATA PRESERVATION GUARANTEE:"
echo "All user state directories (~/.local/share/filesync, ~/.filesync) and all"
echo "synchronized workspace roots have been strictly PRESERVED on disk."
echo "Your files and version histories remain untouched."
echo ""
echo "If you explicitly wish to purge all local sync metadata, you may manually run:"
echo "  rm -rf ~/.local/share/filesync ~/.filesync"
echo "Workspace files in your configured folders will still remain intact."
