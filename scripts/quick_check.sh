#!/usr/bin/env bash
# Trial inner loop (make quick): build, vet, and short tests for only the Go
# packages touched since QUICK_BASE (default HEAD, i.e. uncommitted changes;
# HEAD~1 when the tree is clean), plus the PTY suites the change can affect.
# Not a release gate: run `make check` (or CI) once at the end of a batch.
#
#   QUICK_BASE=main~3 make quick       compare against another revision
#   PTY="onboarding keys" make quick   pick PTY suites (pty onboarding everyday keys | all | none)
set -euo pipefail
cd "$(dirname "$0")/.."
start=$SECONDS
step() { printf '\n== %s (%ss)\n' "$1" "$((SECONDS - start))"; }

base=${QUICK_BASE:-HEAD}
changed=$( (git diff --name-only "$base"; git ls-files --others --exclude-standard) | sort -u)
if [ -z "$changed" ] && [ -z "${QUICK_BASE:-}" ]; then
	base=HEAD~1
	changed=$(git diff --name-only "$base" | sort -u)
fi
echo "changes since $base: $(echo "$changed" | grep -c . || true) file(s)"

# Packages with changed Go files; the slow end-to-end trees run only in make check.
pkgs=$(echo "$changed" | { grep '\.go$' || true; } | grep -E '^(cmd|internal|model)/' | xargs -r -n1 dirname | sort -u |
	while read -r d; do if [ -d "$d" ] && ls "$d"/*.go >/dev/null 2>&1; then echo "./$d"; fi; done || true)
skipped=$(echo "$changed" | { grep -E '^tests/.*\.go$' || true; } | xargs -r -n1 dirname | sort -u | tr '\n' ' ')

step "build"
make --no-print-directory build

step "gofmt"
gofiles=$(echo "$changed" | { grep '\.go$' || true; } | while read -r f; do if [ -f "$f" ]; then echo "$f"; fi; done)
if [ -n "$gofiles" ]; then
	bad=$(gofmt -l $gofiles)
	[ -z "$bad" ] || { echo "needs gofmt: $bad"; exit 1; }
fi

if [ -n "$pkgs" ]; then
	step "vet $(echo $pkgs)"
	go vet $pkgs
	step "test -short $(echo $pkgs)"
	go test -short $pkgs
else
	echo "no Go packages under cmd/ internal/ model/ changed"
fi

# PTY suites: default to all four when terminal-facing code changed.
suites=${PTY:-auto}
if [ "$suites" = auto ]; then
	if echo "$changed" | grep -qE '^(cmd/orbit|internal/(terminal|control|controlclient|launcher))/|^scripts/terminal_.*pty'; then
		suites=all
	else
		suites=none
	fi
fi
[ "$suites" = all ] && suites="pty onboarding everyday keys"
if [ "$suites" != none ]; then
	pids=()
	names=()
	logdir=$(mktemp -d)
	for s in $suites; do
		script=scripts/terminal_${s}_pty_test.py
		[ "$s" = pty ] && script=scripts/terminal_pty_test.py
		python3 "$script" --binary bin/orbit >"$logdir/$s.log" 2>&1 &
		pids+=($!)
		names+=("$s")
	done
	step "PTY suites in parallel: $suites"
	failed=""
	for i in "${!pids[@]}"; do
		if wait "${pids[$i]}"; then echo "  ${names[$i]}: ok"; else
			failed="$failed ${names[$i]}"
			echo "  ${names[$i]}: FAILED"
			tail -25 "$logdir/${names[$i]}.log" | sed 's/^/    /'
		fi
	done
	[ -z "$failed" ] || { echo "PTY logs: $logdir"; exit 1; }
	rm -rf "$logdir"
fi

[ -z "$skipped" ] || echo "not run here (make check): $skipped"
echo "quick check passed in $((SECONDS - start))s"
