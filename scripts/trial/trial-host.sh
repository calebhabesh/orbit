#!/usr/bin/env bash
# Runs on one trial host (copied there by scripts/trial/trial.sh).
#
# The trial is a second, disposable Orbit beside the owner's own. `orbit-trial`
# runs the trial binary with HOME=~/orbit-trial, so its state and its default
# folders (~/<name> in the TUI) all live under ~/orbit-trial. Its unit is
# orbit-trial.service (ORBIT_SERVICE_UNIT); ~/orbit-trial/.config/systemd/user
# links to the real user unit directory so the user manager finds it.
# This script never touches orbit.service, ~/.local/state/orbit, ~/Orbit or the
# Pi's filesync-pilot unit.
set -euo pipefail

LIB=$HOME/.local/lib/orbit-trial
BIN=$LIB/orbit
WRAP=$HOME/.local/bin/orbit-trial
TRIAL=$HOME/orbit-trial
STATE=$TRIAL/.local/state/orbit
MARKER=$TRIAL/.disposable-orbit-trial
UNIT=orbit-trial.service
UNITDIR=$HOME/.config/systemd/user

say() { printf '[%s] %s\n' "$(uname -n)" "$*"; }

unit_active() { systemctl --user is-active -q "$UNIT" 2>/dev/null; }

# PIDs of trial daemons: the trial binary (or a replaced copy) running serve.
trial_pids() {
	local p exe
	for p in /proc/[0-9]*; do
		exe=$(readlink "$p/exe" 2>/dev/null) || continue
		case $exe in "$BIN" | "$BIN (deleted)") ;; *) continue ;; esac
		tr '\0' ' ' <"$p/cmdline" 2>/dev/null | grep -q -- " serve" && echo "${p#/proc/}"
	done
	return 0
}

skeleton() {
	mkdir -p "$TRIAL/.config/systemd" "$UNITDIR"
	chmod 700 "$TRIAL"
	[ -L "$TRIAL/.config/systemd/user" ] || ln -s "$UNITDIR" "$TRIAL/.config/systemd/user"
	touch "$MARKER"
	# The unit's daemon sees the same home as orbit-trial, so paths it proposes
	# (~/<name>) stay inside the trial.
	local dropin=$UNITDIR/$UNIT.d/trial-home.conf want
	want=$(printf '[Service]\nEnvironment=HOME=%%h/orbit-trial XDG_STATE_HOME=%%h/orbit-trial/.local/state ORBIT_TRIAL_REAL_HOME=%%h')
	if [ "$(cat "$dropin" 2>/dev/null)" != "$want" ]; then
		mkdir -p "$(dirname "$dropin")"
		printf '%s\n' "$want" >"$dropin"
		systemctl --user daemon-reload 2>/dev/null || true
	fi
}

stop_daemon() {
	if unit_active; then systemctl --user stop "$UNIT" || true; fi
	if [ -x "$BIN" ] && [ -d "$STATE" ]; then "$BIN" stop --state="$STATE" >/dev/null 2>&1 || true; fi
	local pids
	pids=$(trial_pids)
	if [ -n "$pids" ]; then
		say "stopping leftover trial daemon(s): $pids"
		kill $pids 2>/dev/null || true
		sleep 2
		pids=$(trial_pids)
		[ -z "$pids" ] || kill -9 $pids 2>/dev/null || true
	fi
}

install_bin() {
	local staged=$1
	mkdir -p "$LIB" "$(dirname "$WRAP")"
	skeleton
	install -m 0755 "$staged" "$BIN.new"
	mv -f "$BIN.new" "$BIN"
	cat >"$WRAP.new" <<'EOF'
#!/bin/sh
# Disposable Orbit trial (scripts/trial): its own home, state, folders and unit.
# The real config home is kept so $EDITOR and other tools behave as usual.
real=${ORBIT_TRIAL_REAL_HOME:-$HOME}
export ORBIT_TRIAL_REAL_HOME="$real" HOME="$real/orbit-trial" \
	XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$real/.config}" \
	XDG_STATE_HOME="$real/orbit-trial/.local/state" \
	ORBIT_SERVICE_UNIT=orbit-trial.service
exec "$real/.local/lib/orbit-trial/orbit" "$@"
EOF
	chmod 0755 "$WRAP.new"
	mv -f "$WRAP.new" "$WRAP"
	say "installed $("$BIN" version 2>/dev/null | sed -n '1,2p' | tr '\n' ' ')"
	if unit_active; then
		systemctl --user restart "$UNIT"
		say "restarted $UNIT"
	elif [ -n "$(trial_pids)" ]; then
		stop_daemon
		say "stopped the terminal-started trial daemon; the next orbit-trial command starts the new one"
	fi
}

reset() {
	if [ -d "$TRIAL" ] && [ ! -e "$MARKER" ]; then
		say "refusing: $TRIAL exists without $(basename "$MARKER")"
		exit 1
	fi
	if [ -f "$STATE/config.json" ] && [ -n "$(trial_pids)" ]; then
		# Folders placed outside the trial home are reported, never removed.
		"$WRAP" folders --json 2>/dev/null | python3 -c 'import json,sys
for f in json.load(sys.stdin).get("items",[]): print(f["root"])' 2>/dev/null |
			while IFS= read -r root; do
				case $root in "$TRIAL"/*) ;; *) say "note: trial folder $root is outside $TRIAL; left in place" ;; esac
			done || true
	fi
	stop_daemon
	if [ -e "$UNITDIR/$UNIT" ]; then
		systemctl --user disable "$UNIT" >/dev/null 2>&1 || true
		rm -f "$UNITDIR/$UNIT"
		systemctl --user daemon-reload || true
		systemctl --user reset-failed "$UNIT" 2>/dev/null || true
		say "removed $UNIT"
	fi
	rm -rf -- "$TRIAL"
	skeleton
	say "trial reset: ~/orbit-trial is empty (fresh identity on next setup or join)"
}

status() {
	local v="not installed" d="stopped" u="none" s="not set up"
	[ -x "$BIN" ] && v=$("$BIN" version 2>/dev/null | sed -n '1,2p' | tr '\n' ' ')
	[ -e "$UNITDIR/$UNIT" ] && u="$(systemctl --user is-enabled "$UNIT" 2>/dev/null || true)/$(systemctl --user is-active "$UNIT" 2>/dev/null || true)"
	[ -n "$(trial_pids)" ] && d="running"
	[ -f "$STATE/config.json" ] && s="set up"
	say "$v| daemon $d | unit $u | $s"
	if [ "$s" = "set up" ] && [ "$d" = running ]; then
		"$WRAP" folders 2>/dev/null | sed "s/^/[$(uname -n)]   /" || true
		"$WRAP" devices 2>/dev/null | sed "s/^/[$(uname -n)]   /" || true
	fi
}

case ${1:-} in
install) install_bin "$2" ;;
reset) reset ;;
stop) stop_daemon ;;
status) status ;;
*)
	echo "usage: trial-host.sh install <binary> | reset | stop | status" >&2
	exit 2
	;;
esac
