# T13 commands and evidence index

Run from the repository root unless a different directory is stated. Outputs are
new evidence directories; preserve prior results. Remote harnesses create fresh
private `.filesync-disposable` roots, validate their marker and process ownership,
and stop only their own workers. No personal pilot is a fault target.

## Actual final candidate

```sh
python3 scripts/validation/snapshot_release.py \
  --kernel /boot/vmlinuz-linux \
  --output docs/evidence/terminal-t13-20261004/reproduction-final-candidate
```

This created `/tmp/orbit-t13-snapshot-8lkgsrif/source`, committed
`e37e2600cd236776d5161f3a293718098bd7500e` there, then reproduced from a clean
clone. [Manifest](reproduction-final-candidate/snapshot-manifest.json) records
original dirty source provenance. [Reproduction JSON](reproduction-final-candidate/clean-release/reproduction.json)
is authoritative for exact argv, cwd, timings and outcomes: `make check`, uncached
`go test -race -count=1 ./...`, demo, 16 reset cases, five storage cases, checksums
and identical repeat packages all passed. The original branch/index was untouched.

The matching snapshot also ran `make package`, writing `source/dist`. All native
LAN/service/engine binaries match those package payload SHA-256 values and the
clean reproduction's packages, as verified in [results](results.json). The
engine runner's `bin/filesync` and `bin/filesync-linux-arm64` were those matching
candidate binaries; a build label alone is not the provenance check.

## Packet and native checks

Reproduction entry points for the completed campaigns are below. Native reports
record actual addresses, root paths, times and payload hashes; the explicit LAN
address arguments make that route assumption visible when repeating the run.

```sh
GOFLAGS=-race go test -race -count=2 -v ./tests/terminal -run '^TestTerminalT13'
go test -count=2 -v ./tests/terminal -run '^TestTerminalT13'
python3 scripts/validation/terminal_native.py \
  --dist /tmp/orbit-t13-snapshot-8lkgsrif/source/dist \
  --hosts laptop rpi --addresses 192.168.88.83 192.168.88.63 \
  --output docs/evidence/terminal-t13-20261004/lan-final-candidate
python3 scripts/validation/terminal_hosts.py \
  --dist /tmp/orbit-t13-snapshot-8lkgsrif/source/dist \
  --hosts rpi vps laptop \
  --output docs/evidence/terminal-t13-20261004/native-hosts-final-candidate-03
python3 scripts/validation/three_host.py \
  --output docs/evidence/terminal-t13-20261004/native-engine-final-candidate
```

Six packet cases passed twice in each mode, with spawned binaries instrumented
in the race run. Logs: [race](final-candidate-packet-race.log),
[ordinary](final-candidate-packet-normal.log). `make test-terminal-release` is the
persistent instrumented target. Additional repeated native-fix regressions are
in [native-fixes-regressions.log](native-fixes-regressions.log).

Native outputs: [ordinary LAN](lan-final-candidate/terminal-native.json),
[keyboard/service](native-hosts-final-candidate-03/terminal-hosts.json),
[three-host engine](native-engine-final-candidate/three-host.json).
All report success. The engine runner uses manual membership/SSH relays;
the LAN runner uses ordinary reviewed setup and direct Orbit packets.
Service checks use unique units and do not reboot or change lingering policy.
Local current-fixture PTYs passed separately:
[shell](final-shell-pty/results.json), [everyday](final-everyday-pty/results.json).

## Package transactions and dependency checks

```sh
python3 scripts/terminal_package_test.py \
  --dist /tmp/orbit-t13-snapshot-8lkgsrif/source/dist \
  --containers --emulate-arm64 \
  --output docs/evidence/terminal-t13-20261004/final-candidate-package-transactions.json
python3 -m unittest discover -s scripts/validation -p 'test_*.py'
go mod verify
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
git diff --check
```

All 11 [package scenarios](final-candidate-package-transactions.json) passed;
arm64 emulation and container systemd limitations are explicit. The Python
suite has [ten passing safety/VT tests](final-python-safety-vt.log). Dependency
[verification](go-mod-verify.log) and [scan](govulncheck.log) passed; the latter
is a point-in-time vulnerability result. Final documentation links, source/
package hash reconciliation and diff validation are recorded in `handoff-checks.log`.

## Repeating the campaign

The original workspace still has development changes. For a fresh candidate,
use `snapshot_release.py` with a **new** output path, then use its printed
snapshot `dist` for the native and package commands. For an already committed
candidate, use `reproduce_release.py --source <commit> --kernel <kernel> --output
<new-output>` in that repository. Native SSH aliases must identify the intended
hosts. Use actual existing reachable LAN/Tailscale addresses; the runner does
not provision or authenticate a VPN. Manual engine fixtures cannot replace
ordinary terminal onboarding.

Two PTY waits were corrected after the final production snapshot. Exact runner
hashes and production equality are in [current-source-audit.json](current-source-audit.json).
Native and local PTYs ran those current fixtures. Full Go/VM/package checks use
the final candidate. Evidence/docs assembled later are not falsely described
as being in that snapshot.

Existing authenticated Tailscale, ordinary private-route native three-host
onboarding, and native login/logout/boot remain unexecuted. See the
[report](summary.md#remaining-technical-acceptance). Owner review is deferred.
