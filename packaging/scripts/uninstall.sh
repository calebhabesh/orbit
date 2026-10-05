#!/usr/bin/env bash
# Orbit and File Sync uninstallation script
set -euo pipefail

MODE="${1:-user}"
case "$MODE" in user|system) ;; *) echo "Usage: uninstall.sh [user|system]" >&2; exit 1 ;; esac

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

if [ "$MODE" = user ]; then
for f in "${USER_BIN}" "${USER_ORBIT_BIN}" "${USER_SERVICE}" "${USER_ORBIT_SERVICE}" "${USER_DESKTOP}" "${USER_ICON}"; do
    if [ -f "${f}" ] || [ -L "${f}" ]; then
        rm -f "${f}"
        echo "Removed ${f}"
    fi
done

fi

# Remove system-wide installation if present and running with permissions
SYS_BIN="/usr/local/bin/filesync"
SYS_ORBIT_BIN="/usr/local/bin/orbit"
SYS_SERVICE="/usr/lib/systemd/user/filesync.service"
SYS_ORBIT_SERVICE="/usr/lib/systemd/user/orbit.service"
SYS_DESKTOP="/usr/local/share/applications/orbit.desktop"
SYS_ICON="/usr/local/share/icons/hicolor/scalable/apps/orbit.svg"

if [ "$MODE" = system ]; then
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

fi

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
# Remove only distributed completion/runbook artifacts, never state or roots.
SHARE_DIR="${HOME}/.local/share"
if [ "$MODE" = system ]; then SHARE_DIR=/usr/local/share; fi
for share in "$SHARE_DIR"; do
    for entry in bash-completion/completions/orbit zsh/site-functions/_orbit fish/vendor_completions.d/orbit.fish; do
        if [ -w "$(dirname "$share/$entry")" ]; then rm -f "$share/$entry"; fi
    done
    for entry in LICENSE NOTICE LICENSES.md README.md release-manifest.json; do
        if [ -w "$share/doc/filesync" ]; then rm -f "$share/doc/filesync/$entry"; fi
    done
    if [ -d "$share/doc/filesync/runbooks" ] && [ -w "$share/doc/filesync/runbooks" ]; then
        SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
        for bundled in "$SCRIPT_DIR/share/doc/filesync/runbooks/"*.md; do
            if [ -f "$bundled" ]; then rm -f "$share/doc/filesync/runbooks/$(basename "$bundled")"; fi
        done
        rmdir "$share/doc/filesync/runbooks" 2>/dev/null || true
    fi
done
