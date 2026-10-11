#!/usr/bin/env bash
# R04/RG3: a stored release selection made by an older build survives an
# upgrade to a public build without a packaged profile, and a rollback.
# Runs inside one empty network namespace (unshare -rn: loopback only), so no
# daemon can reach any Orbit service. Usage:
#   scripts/validation/r04_upgrade_rollback.sh LEGACY_ORBIT PUBLIC_ORBIT NEW_ROOT
set -euo pipefail
if [[ "${R04_INNER:-}" != 1 ]]; then
	[[ $# -eq 3 && ! -e "$3" ]] || { echo "usage: $0 LEGACY PUBLIC NEW_ROOT (must not exist)" >&2; exit 2; }
	mkdir -m 0700 "$3"; echo "R04 disposable" > "$3/.orbit-r04-disposable"
	R04_INNER=1 exec unshare -rn "$0" "$@"
fi
L=$1 P=$2 ROOT=$3
[[ -f "$ROOT/.orbit-r04-disposable" ]] || { echo "refusing: unmarked root" >&2; exit 2; }
export HOME=$ROOT/home XDG_CONFIG_HOME=$ROOT/home/.config XDG_STATE_HOME=$ROOT/home/.local/state
mkdir -m 0700 -p "$HOME"
S=$ROOT/state
step() { echo "== $*"; }
field() { python3 -c "import json,sys;n=json.load(sys.stdin)['network'];print(' '.join(str(n.get(k) if k in n else n['policy'].get(k)) for k in sys.argv[1:]))" "$@"; }
ip link set lo up
step "network interfaces (expect loopback only)"; ip -brief link; ip route
step "legacy: adopt the packaged release profile (Automatic)"
"$L" network automatic --yes --state "$S" --json >/dev/null 2>&1 || true
"$L" network status --state "$S" --json | field mode profile_state operator code
DEVICE=$(python3 -c "import json;print(json.load(open('$S/config.json'))['device_id'])")
"$L" stop --state "$S" >/dev/null 2>&1 || true
step "public build: packaged profile"
"$P" version --json | python3 -c "import json,sys;print(json.load(sys.stdin)['packaged_profile'])"
step "public build: stored selection after upgrade"
"$P" serve --state "$S" --control-listen 127.0.0.1:0 >"$ROOT/public.log" 2>&1 &
for _ in $(seq 100); do [[ -f $S/control.addr ]] && "$P" network status --state "$S" --json >/dev/null 2>&1 && break; sleep 0.2; done
sleep 2
"$P" network status --state "$S" --json | field mode profile_state operator code profile
"$P" stop --state "$S" >/dev/null 2>&1 || kill %1 || true
wait || true
step "rollback: legacy build opens the same state"
"$L" network status --state "$S" --json | field mode profile_state operator code
AFTER=$(python3 -c "import json;print(json.load(open('$S/config.json'))['device_id'])")
[[ "$DEVICE" == "$AFTER" ]] && echo "device identity unchanged"
"$L" stop --state "$S" >/dev/null 2>&1 || true
