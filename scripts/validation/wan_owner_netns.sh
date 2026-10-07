#!/usr/bin/env bash
# W16 owner-run namespace for a host where the runner has no sudo (the laptop).
# Run once by the owner; everything later runs as USER through the shell socket.
#
#   sudo ORBIT_W16_DISPOSABLE=1 wan_owner_netns.sh up   USER UPLINK OCTET
#   sudo ORBIT_W16_DISPOSABLE=1 wan_owner_netns.sh down USER UPLINK OCTET
#
# up: records net.ipv4.ip_forward, enables it at runtime only, runs
# wan_netns.sh up w16, saves the tagged rules plus host links to
# ~USER/orbit-w16/netns-snapshot.txt, and starts wan_netns_shell.py as USER
# inside orbit-w16 on ~USER/orbit-w16/run/shell.sock (exits after 4 h idle).
# down: stops that shell, runs wan_netns.sh down w16 and restores ip_forward.
# Nothing is written to /etc/sysctl* or saved firewall files.
set -euo pipefail
[[ $# -eq 4 ]] || { sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }
action=$1 user=$2 uplink=$3 octet=$4
[[ ${ORBIT_W16_DISPOSABLE:-} == 1 ]] || { echo "refusing: set ORBIT_W16_DISPOSABLE=1" >&2; exit 3; }
[[ $user =~ ^[a-z_][a-z0-9_-]{0,31}$ ]] && id "$user" >/dev/null || { echo "unknown user" >&2; exit 2; }
here=$(cd "$(dirname "$0")" && pwd)
home=$(getent passwd "$user" | cut -d: -f6)
dir="$home/orbit-w16" run="$home/orbit-w16/run"
saved="$dir/ip_forward.before"

case $action in
up)
	[[ -f $saved ]] || sysctl -n net.ipv4.ip_forward > "$saved"
	sysctl -qw net.ipv4.ip_forward=1
	ORBIT_W16_DISPOSABLE=1 "$here/wan_netns.sh" up w16 "$uplink" "$octet"
	{ iptables -w -S -v; iptables -w -t nat -S -v; echo @@LINKS; ip -j -d link; } > "$dir/netns-snapshot.txt"
	chown "$user:" "$dir/netns-snapshot.txt" "$saved"
	install -d -m 0700 -o "$user" -g "$(id -gn "$user")" "$run"
	# A new session without a terminal: closing the owner's SSH login cannot hang it up.
	setsid -f ip netns exec orbit-w16 sudo -u "$user" -H python3 "$here/wan_netns_shell.py" "$run/shell.sock" \
		</dev/null >"$run/shell.log" 2>&1
	for _ in $(seq 50); do [[ -S $run/shell.sock ]] && break; sleep 0.1; done
	[[ -S $run/shell.sock ]] || { echo "shell did not start; see $run/shell.log" >&2; exit 1; }
	echo "shell: $run/shell.sock"
	;;
down)
	pkill -u "$user" -f "wan_netns_shell.py $run/shell.sock" || true
	ORBIT_W16_DISPOSABLE=1 "$here/wan_netns.sh" down w16 "$uplink" "$octet"
	if [[ -f $saved ]]; then sysctl -qw net.ipv4.ip_forward="$(cat "$saved")"; rm -f "$saved"; fi
	echo "ip_forward=$(sysctl -n net.ipv4.ip_forward)"
	;;
*) echo "unknown action" >&2; exit 2 ;;
esac
