#!/usr/bin/env bash
# Orbit and File Sync uninstallation script
set -euo pipefail

echo "Stopping and disabling Orbit / filesync systemd user service..."
systemctl --user stop orbit.service filesync.service 2>/dev/null || true
systemctl --user disable orbit.service filesync.service 2>/dev/null || true

# Remove user-local installation if present
USER_BIN="${HOME}/.local/bin/filesync"
USER_ORBIT_BIN="${HOME}/.local/bin/orbit"
USER_SERVICE="${HOME}/.config/systemd/user/filesync.service"
USER_ORBIT_SERVICE="${HOME}/.config/systemd/user/orbit.service"
USER_DESKTOP="${HOME}/.local/share/applications/orbit.desktop"
USER_ICON="${HOME}/.local/share/icons/hicolor/scalable/apps/orbit.svg"

for f in "${USER_BIN}" "${USER_ORBIT_BIN}" "${USER_SERVICE}" "${USER_ORBIT_SERVICE}" "${USER_DESKTOP}" "${USER_ICON}"; do
    if [ -f "${f}" ] || [ -L "${f}" ]; then
        rm -f "${f}"
        echo "Removed ${f}"
    fi
done

# Remove system-wide installation if present and running with permissions
SYS_BIN="/usr/local/bin/filesync"
SYS_ORBIT_BIN="/usr/local/bin/orbit"
SYS_SERVICE="/usr/lib/systemd/user/filesync.service"
SYS_ORBIT_SERVICE="/usr/lib/systemd/user/orbit.service"
SYS_DESKTOP="/usr/local/share/applications/orbit.desktop"
SYS_ICON="/usr/local/share/icons/hicolor/scalable/apps/orbit.svg"

for f in "${SYS_BIN}" "${SYS_ORBIT_BIN}" "${SYS_SERVICE}" "${SYS_ORBIT_SERVICE}" "${SYS_DESKTOP}" "${SYS_ICON}"; do
    if [ -f "${f}" ] || [ -L "${f}" ]; then
        if [ -w "$(dirname "${f}")" ]; then
            rm -f "${f}"
            echo "Removed ${f}"
        else
            echo "Notice: ${f} requires root permissions to remove (sudo rm -f ${f})"
        fi
    fi
done

systemctl --user daemon-reload 2>/dev/null || true

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "${HOME}/.local/share/applications" 2>/dev/null || true
fi

echo ""
echo "Uninstallation complete."
echo "DATA PRESERVATION GUARANTEE (Invariant S21 / I20):"
echo "All user state directories (~/.local/state/filesync, ~/.filesync) and all"
echo "synchronized workspace roots have been strictly PRESERVED on disk."
echo "Your files and version histories remain untouched."
echo ""
echo "If you explicitly wish to purge all local sync metadata, you may manually run:"
echo "  rm -rf ~/.local/state/filesync ~/.filesync"
echo "Workspace files in your configured folders will still remain intact."
