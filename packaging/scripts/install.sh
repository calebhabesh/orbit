#!/usr/bin/env bash
# File Sync standalone installation script
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_SRC="${SCRIPT_DIR}/filesync"
SERVICE_SRC="${SCRIPT_DIR}/systemd/filesync.service"

if [ ! -f "${BIN_SRC}" ]; then
    echo "Error: filesync binary not found in ${SCRIPT_DIR}" >&2
    exit 1
fi

MODE="${1:-user}"

if [ "${MODE}" = "system" ]; then
    echo "Installing File Sync system-wide (requires root)..."
    INSTALL_BIN="/usr/local/bin/filesync"
    INSTALL_SERVICE="/usr/lib/systemd/user/filesync.service"

    install -d -m 0755 /usr/local/bin
    install -m 0755 "${BIN_SRC}" "${INSTALL_BIN}"

    install -d -m 0755 /usr/lib/systemd/user
    install -m 0644 "${SERVICE_SRC}" "${INSTALL_SERVICE}"

    echo "Installed binary: ${INSTALL_BIN}"
    echo "Installed service: ${INSTALL_SERVICE}"
else
    echo "Installing File Sync for current user (${USER})..."
    TARGET_BIN_DIR="${HOME}/.local/bin"
    TARGET_SERVICE_DIR="${HOME}/.config/systemd/user"

    mkdir -p "${TARGET_BIN_DIR}"
    install -m 0755 "${BIN_SRC}" "${TARGET_BIN_DIR}/filesync"

    mkdir -p "${TARGET_SERVICE_DIR}"
    # Adjust service file to point to ~/.local/bin/filesync if user install
    sed "s|/usr/bin/filesync|${TARGET_BIN_DIR}/filesync|g" "${SERVICE_SRC}" > "${TARGET_SERVICE_DIR}/filesync.service"
    chmod 0644 "${TARGET_SERVICE_DIR}/filesync.service"

    echo "Installed binary: ${TARGET_BIN_DIR}/filesync"
    echo "Installed service: ${TARGET_SERVICE_DIR}/filesync.service"

    # Check PATH
    if [[ ":$PATH:" != *":${TARGET_BIN_DIR}:"* ]]; then
        echo "Notice: ${TARGET_BIN_DIR} is not currently in your PATH."
        echo "Add 'export PATH=\"\$HOME/.local/bin:\$PATH\"' to your ~/.bashrc or ~/.profile."
    fi
fi

systemctl --user daemon-reload || true

echo ""
echo "Installation complete!"
echo "Next steps:"
echo "1. Initialize your device: filesync init"
echo "2. Enable session lingering so the daemon runs across logout/boot: loginctl enable-linger ${USER}"
echo "3. Enable and start the background sync service: systemctl --user enable --now filesync.service"
echo "4. Open the operator console at http://127.0.0.1:8080 (or generate a bootstrap token with 'filesync control bootstrap-token')"
