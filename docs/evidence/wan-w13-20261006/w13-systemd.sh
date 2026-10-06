#!/bin/bash
# Run the packaged binary under the unit's sandbox properties that a user
# manager can apply (capabilities/User= need the system manager).
set -euo pipefail
ARCHIVE="$1"; IP="$2"; SCRATCH="$3"
ROOT=$(mktemp -d "$SCRATCH/orbit-w13-systemd-XXXXXX"); chmod 700 "$ROOT"; echo "filesync test data only" > "$ROOT/.filesync-disposable"
UNIT=orbit-net-w13-$$
trap 'systemctl --user stop $UNIT 2>/dev/null || true; [ -f "$ROOT/.filesync-disposable" ] && rm -rf "$ROOT"' EXIT
cd "$ROOT"; tar -xzf "$ARCHIVE"; BIN=$(echo "$ROOT"/orbit-net-*/bin/orbit-net); mkdir -m 700 op op/tls
PORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("'"$IP"'",0));print(s.getsockname()[1])')
MPORT=$(python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])')
SVC=$("$BIN" keygen --out op/service.key); "$BIN" keygen --out op/authority.key >/dev/null
printf '{"version":"1","operator":"W13 systemd","service_key":"%s","epoch":"1","origins":["https://%s:%s","wss://%s:%s"],"stun":[],"privacy":"Sandbox check only."}' "$SVC" "$IP" "$PORT" "$IP" "$PORT" > op/template.json
"$BIN" profile sign --authority-key op/authority.key --template op/template.json --environment self_hosted --valid-for 1h --out op/profile.json >/dev/null
openssl genpkey -algorithm ed25519 -out op/tls/privkey.pem 2>/dev/null
openssl req -x509 -new -key op/tls/privkey.pem -subj "/CN=$IP" -days 2 -addext "subjectAltName=IP:$IP" -addext basicConstraints=critical,CA:TRUE -out op/tls/fullchain.pem 2>/dev/null
chmod 600 op/tls/*
printf '{"listen":"%s:%s","profile":"%s","service_key":"%s","tls_cert":"%s","tls_key":"%s","metrics_listen":"127.0.0.1:%s","relay_bps":8388608,"relay_device_bps":4194304,"relay_session_bytes":1073741824}' "$IP" "$PORT" "$ROOT/op/profile.json" "$ROOT/op/service.key" "$ROOT/op/tls/fullchain.pem" "$ROOT/op/tls/privkey.pem" "$MPORT" > op/serve.json; chmod 600 op/serve.json
PROPS=(-p NoNewPrivileges=yes -p SystemCallFilter=@system-service -p SystemCallArchitectures=native -p MemoryDenyWriteExecute=yes -p "RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX" -p LockPersonality=yes -p RestrictRealtime=yes -p RestrictSUIDSGID=yes -p RestrictNamespaces=yes -p UMask=0077 -p MemoryHigh=384M -p MemoryMax=512M -p TasksMax=512 -p LimitNOFILE=4096 -p KillSignal=SIGTERM -p TimeoutStopSec=15s)
systemd-run --user --quiet --wait --collect "${PROPS[@]}" "$BIN" serve --config "$ROOT/op/serve.json" --check && echo "sandboxed --check: ok"
systemd-run --user --quiet --unit=$UNIT "${PROPS[@]}" "$BIN" serve --config "$ROOT/op/serve.json"
for i in $(seq 50); do curl -fsS "http://127.0.0.1:$MPORT/healthz" >/dev/null 2>&1 && break; sleep 0.1; done
echo "sandboxed healthz: $(curl -fsS http://127.0.0.1:$MPORT/healthz)"
echo "TLS: $(curl -sS --cacert op/tls/fullchain.pem -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{}' https://$IP:$PORT/network/v1/challenge) (400 expected for an invalid body)"
PID=$(systemctl --user show -p MainPID --value $UNIT); kill -HUP "$PID"; sleep 0.5
systemctl --user is-active $UNIT
systemctl --user stop $UNIT
journalctl --user -u $UNIT --no-pager -o cat | grep orbit-net: | grep -v warning
systemctl --user show -p Result --value $UNIT 2>/dev/null || true
