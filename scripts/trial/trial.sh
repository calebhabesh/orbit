#!/usr/bin/env bash
# Fast owner trial loop: build once, then install, reset or inspect the
# disposable `orbit-trial` instance on every trial host in parallel.
#
#   scripts/trial/trial.sh install [host...]   build and install (restarts a running trial)
#   scripts/trial/trial.sh reset [host...]     wipe trial state, unit and trial folders
#   scripts/trial/trial.sh fresh [host...]     reset, then install
#   scripts/trial/trial.sh status [host...]
#   scripts/trial/trial.sh stop [host...]
#
# Hosts default to $TRIAL_HOSTS or "local laptop rpi"; others are ssh names.
# The owner's own orbit/orbit.service, ~/Orbit and the Pi pilot are untouched;
# see trial-host.sh for what the trial owns.
set -euo pipefail

repo=$(cd "$(dirname "$0")/../.." && pwd)
cmd=${1:-}
[ $# -gt 0 ] && shift
hosts=("$@")
[ ${#hosts[@]} -gt 0 ] || read -r -a hosts <<<"${TRIAL_HOSTS:-local laptop rpi}"

out=$repo/bin/trial
ctl=${XDG_RUNTIME_DIR:-/tmp}/orbit-trial-ssh-%r@%h
ssh_opts=(-o BatchMode=yes -o ConnectTimeout=8 -o ControlMaster=auto -o "ControlPath=$ctl" -o ControlPersist=120)

host_arch() {
	local m
	if [ "$1" = local ]; then m=$(uname -m); else m=$(ssh "${ssh_opts[@]}" "$1" uname -m); fi
	case $m in x86_64) echo amd64 ;; aarch64 | arm64) echo arm64 ;; *) echo "unsupported arch $m on $1" >&2 && return 1 ;; esac
}

build() {
	local arch=$1 commit version
	commit=$(git -C "$repo" rev-parse --short HEAD)
	[ -z "$(git -C "$repo" status --porcelain --untracked-files=no)" ] || commit="$commit-dirty"
	version=$(sed -n 's/^VERSION ?= //p' "$repo/Makefile")
	mkdir -p "$out"
	(cd "$repo" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
		-ldflags "-X main.version=$version -X main.commit=$commit -X main.date=$(date +%F)" \
		-o "$out/orbit-$arch" ./cmd/orbit)
}

# run_host HOST ACTION [BINARY]: copy the helper (and binary), then run it.
run_host() {
	local host=$1 action=$2 bin=${3:-}
	if [ "$host" = local ]; then
		local stage=$HOME/.local/lib/orbit-trial/stage
		mkdir -p "$stage"
		install -m 0755 "$repo/scripts/trial/trial-host.sh" "$stage/trial-host.sh"
		if [ -n "$bin" ]; then
			install -m 0755 "$bin" "$stage/orbit"
			"$stage/trial-host.sh" "$action" "$stage/orbit"
		else
			"$stage/trial-host.sh" "$action"
		fi
		return
	fi
	ssh "${ssh_opts[@]}" "$host" 'mkdir -p ~/.local/lib/orbit-trial/stage'
	local files=("$repo/scripts/trial/trial-host.sh")
	[ -z "$bin" ] || files+=("$bin")
	scp -q "${ssh_opts[@]}" "${files[@]}" "$host:.local/lib/orbit-trial/stage/"
	if [ -n "$bin" ]; then
		ssh "${ssh_opts[@]}" "$host" "bash ~/.local/lib/orbit-trial/stage/trial-host.sh $action ~/.local/lib/orbit-trial/stage/$(basename "$bin")"
	else
		ssh "${ssh_opts[@]}" "$host" "bash ~/.local/lib/orbit-trial/stage/trial-host.sh $action"
	fi
}

# on_all ACTION [with-binary]: run on every host in parallel, fail if any fails.
on_all() {
	local action=$1 with_bin=${2:-} pids=() host arch failed=0
	declare -A archs=()
	for host in "${hosts[@]}"; do
		if [ -n "$with_bin" ]; then
			arch=$(host_arch "$host")
			archs[$host]=$arch
		fi
	done
	if [ -n "$with_bin" ]; then
		for arch in $(printf '%s\n' "${archs[@]}" | sort -u); do
			echo "building linux/$arch"
			build "$arch"
		done
	fi
	for host in "${hosts[@]}"; do
		if [ -n "$with_bin" ]; then
			run_host "$host" "$action" "$out/orbit-${archs[$host]}" &
		else
			run_host "$host" "$action" &
		fi
		pids+=($!)
	done
	for i in "${!pids[@]}"; do
		wait "${pids[$i]}" || { echo "FAILED on ${hosts[$i]}" >&2; failed=1; }
	done
	return $failed
}

start=$SECONDS
case $cmd in
install) on_all install with-bin ;;
reset) on_all reset ;;
fresh) on_all reset && on_all install with-bin ;;
status) on_all status ;;
stop) on_all stop ;;
*)
	sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
	;;
esac
echo "done in $((SECONDS - start))s on: ${hosts[*]}"
