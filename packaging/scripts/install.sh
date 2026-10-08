#!/usr/bin/env bash
# Orbit and Orbit standalone installation script
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_SRC="${SCRIPT_DIR}/orbit"
SERVICE_SRC="${SCRIPT_DIR}/systemd/orbit.service"
DESKTOP_SRC="${SCRIPT_DIR}/desktop/orbit.desktop"
ICON_SRC="${SCRIPT_DIR}/icons/orbit.svg"

# Fall back to packaging/ and bin/ paths if running from repository root or scripts dir
if [ ! -f "${BIN_SRC}" ] && [ -f "${SCRIPT_DIR}/../../bin/orbit" ]; then
    BIN_SRC="${SCRIPT_DIR}/../../bin/orbit"
fi
if [ ! -f "${BIN_SRC}" ] && [ -f "${SCRIPT_DIR}/bin/orbit" ]; then
    BIN_SRC="${SCRIPT_DIR}/bin/orbit"
fi
if [ ! -f "${SERVICE_SRC}" ] && [ -f "${SCRIPT_DIR}/../systemd/orbit.service" ]; then
    SERVICE_SRC="${SCRIPT_DIR}/../systemd/orbit.service"
fi
if [ ! -f "${SERVICE_SRC}" ] && [ -f "${SCRIPT_DIR}/packaging/systemd/orbit.service" ]; then
    SERVICE_SRC="${SCRIPT_DIR}/packaging/systemd/orbit.service"
fi
if [ ! -f "${DESKTOP_SRC}" ] && [ -f "${SCRIPT_DIR}/../desktop/orbit.desktop" ]; then
    DESKTOP_SRC="${SCRIPT_DIR}/../desktop/orbit.desktop"
fi
if [ ! -f "${DESKTOP_SRC}" ] && [ -f "${SCRIPT_DIR}/packaging/desktop/orbit.desktop" ]; then
    DESKTOP_SRC="${SCRIPT_DIR}/packaging/desktop/orbit.desktop"
fi
if [ ! -f "${ICON_SRC}" ] && [ -f "${SCRIPT_DIR}/../icons/orbit.svg" ]; then
    ICON_SRC="${SCRIPT_DIR}/../icons/orbit.svg"
fi
if [ ! -f "${ICON_SRC}" ] && [ -f "${SCRIPT_DIR}/packaging/icons/orbit.svg" ]; then
    ICON_SRC="${SCRIPT_DIR}/packaging/icons/orbit.svg"
fi

if [ ! -f "${BIN_SRC}" ]; then
    echo "Error: orbit binary not found in ${SCRIPT_DIR}" >&2
    exit 1
fi

MODE="${1:-user}"
case "$MODE" in user|system) ;; *) echo "Usage: install.sh [user|system]" >&2; exit 1 ;; esac

if [ "${MODE}" = "system" ]; then
    echo "Installing Orbit system-wide (requires root)..."
    INSTALL_BIN="/usr/local/bin/orbit"
    INSTALL_SERVICE="/usr/lib/systemd/user/orbit.service"
    INSTALL_APP_DIR="/usr/local/share/applications"
    INSTALL_ICON_DIR="/usr/local/share/icons/hicolor/scalable/apps"

    install -d -m 0755 /usr/local/bin
    install -m 0755 "${BIN_SRC}" "${INSTALL_BIN}"

    install -d -m 0755 /usr/lib/systemd/user
    if [ -f "${SERVICE_SRC}" ] && [ ! -e "${INSTALL_SERVICE}" ] && [ ! -L "${INSTALL_SERVICE}" ]; then
        sed "s|/usr/bin/orbit|${INSTALL_BIN}|g" "${SERVICE_SRC}" > "${INSTALL_SERVICE}"
        chmod 0644 "${INSTALL_SERVICE}"
    fi

    if [ -f "${DESKTOP_SRC}" ]; then
        install -d -m 0755 "${INSTALL_APP_DIR}"
        sed "s|Exec=orbit$|Exec=${INSTALL_BIN}|g" "${DESKTOP_SRC}" > "${INSTALL_APP_DIR}/orbit.desktop"
        chmod 0644 "${INSTALL_APP_DIR}/orbit.desktop"
    fi

    if [ -f "${ICON_SRC}" ]; then
        install -d -m 0755 "${INSTALL_ICON_DIR}"
        install -m 0644 "${ICON_SRC}" "${INSTALL_ICON_DIR}/orbit.svg"
    fi

    echo "Installed binary: ${INSTALL_BIN}"
    echo "Installed service: ${INSTALL_SERVICE}"
    echo "Installed desktop launcher: ${INSTALL_APP_DIR}/orbit.desktop"
else
    echo "Installing Orbit for current user (${USER})..."
    TARGET_BIN_DIR="${HOME}/.local/bin"
    TARGET_SERVICE_DIR="${HOME}/.config/systemd/user"
    TARGET_APP_DIR="${HOME}/.local/share/applications"
    TARGET_ICON_DIR="${HOME}/.local/share/icons/hicolor/scalable/apps"

    mkdir -p "${TARGET_BIN_DIR}"
    install -m 0755 "${BIN_SRC}" "${TARGET_BIN_DIR}/orbit"

    mkdir -p "${TARGET_SERVICE_DIR}"
    if [ -f "${SERVICE_SRC}" ] && [ ! -e "${TARGET_SERVICE_DIR}/orbit.service" ] && [ ! -L "${TARGET_SERVICE_DIR}/orbit.service" ]; then
        sed "s|/usr/bin/orbit|${TARGET_BIN_DIR}/orbit|g" "${SERVICE_SRC}" > "${TARGET_SERVICE_DIR}/orbit.service"
        chmod 0644 "${TARGET_SERVICE_DIR}/orbit.service"
    fi

    if [ -f "${DESKTOP_SRC}" ]; then
        mkdir -p "${TARGET_APP_DIR}"
        sed "s|Exec=orbit$|Exec=${TARGET_BIN_DIR}/orbit|g" "${DESKTOP_SRC}" > "${TARGET_APP_DIR}/orbit.desktop"
        chmod 0644 "${TARGET_APP_DIR}/orbit.desktop"
    fi

    if [ -f "${ICON_SRC}" ]; then
        mkdir -p "${TARGET_ICON_DIR}"
        install -m 0644 "${ICON_SRC}" "${TARGET_ICON_DIR}/orbit.svg"
    fi

    echo "Installed binary: ${TARGET_BIN_DIR}/orbit"
    echo "Installed service: ${TARGET_SERVICE_DIR}/orbit.service"
    echo "Installed desktop launcher: ${TARGET_APP_DIR}/orbit.desktop"

    # Check PATH
    if [[ ":$PATH:" != *":${TARGET_BIN_DIR}:"* ]]; then
        echo "Notice: ${TARGET_BIN_DIR} is not currently in your PATH."
        echo "Add 'export PATH=\"\$HOME/.local/bin:\$PATH\"' to your ~/.bashrc or ~/.profile."
    fi
fi

# Install completions and runbooks alongside the selected installation.
SHARE_SRC="${SCRIPT_DIR}/share"
if [ -d "$SHARE_SRC" ]; then
    if [ "$MODE" = system ]; then TARGET_SHARE="/usr/local/share"; else TARGET_SHARE="${HOME}/.local/share"; fi
    for entry in bash-completion/completions/orbit zsh/site-functions/_orbit fish/vendor_completions.d/orbit.fish; do
        install -D -m 0644 "$SHARE_SRC/$entry" "$TARGET_SHARE/$entry"
    done
    mkdir -p "$TARGET_SHARE/doc/orbit/runbooks"
    install -m 0644 "$SHARE_SRC"/doc/orbit/runbooks/*.md "$TARGET_SHARE/doc/orbit/runbooks/"
    for entry in LICENSE NOTICE LICENSES.md README.md release-manifest.json; do
        install -m 0644 "$SCRIPT_DIR/$entry" "$TARGET_SHARE/doc/orbit/$entry"
    done
fi

# Reload systemd user daemon and smoothly adopt existing service if running (G05 policy)
if command -v systemctl >/dev/null 2>&1; then
    systemctl --user daemon-reload 2>/dev/null || true
    if systemctl --user is-active --quiet orbit.service 2>/dev/null; then
        echo "Existing orbit service is active; reloading unit..."
        systemctl --user try-restart orbit.service 2>/dev/null || true
    elif systemctl --user is-active --quiet orbit.service 2>/dev/null; then
        echo "Existing orbit service is active; reloading unit..."
        systemctl --user try-restart orbit.service 2>/dev/null || true
    fi
fi

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "${TARGET_APP_DIR:-/usr/local/share/applications}" 2>/dev/null || true
fi

echo ""
echo "Installation complete!"
echo "Next steps:"
echo "1. Launch Orbit directly: orbit (TTY) or orbit status (scripts)"
echo "2. Check service and status: orbit status"
echo "3. Choose manual, login, or unattended startup; see the installed runbooks."
echo "4. Review existing state/units before enabling startup; no service is enabled by installation."
