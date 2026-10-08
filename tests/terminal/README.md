# Terminal baseline reproductions

T00 exercises the current production HTTP handlers, real repository/workspace
operations and a real CLI/daemon child in fresh marked temporary roots. Generated
keys, synthetic capabilities and local credentials stay out of transcripts.
No personal roots, user services or host network policies are changed.

Discover the tests before running them:

```sh
go test ./... -list '^TestTerminalT00'
ORBIT_TERMINAL_BASELINE=1 go test -count=2 -v ./tests/terminal -run '^TestTerminalT00'
```

The opt-in command deliberately exits nonzero on the current implementation.
Assertions describe approved behavior, not an expectation that the bug persists.
Ordinary runs skip these cases. T02–T08 must promote the relevant cases into
passing, normally enabled regressions when repairing their owning behavior.
The T01 contracts may change provisional field names used for absent observations.

The CLI case builds a binary named `orbit` so it exercises the actual executable
dispatch. It starts the daemon with the current default launcher's arguments,
retains the direct child handle, and validates its marked state root and `/proc`
arguments before graceful SIGTERM and waiting. It does not signal a discovered
daemon. Database reopen models persisted state resumption; it is not crash or
power-loss evidence. The TLS identity case uses an unrelated self-signed fixture
server to detect capability disclosure; it does not claim a deployed exploit.

Evidence, dispositions, packet ownership and the source-only cases are recorded
in [the T00 report](../../docs/evidence/terminal-t00-20261003/summary.md).
Run broad gates sequentially: packaging tests and `make check` write the same
`dist` artifacts, so overlapping them can invalidate packaging observations.

T12 adds bare entry/legacy discovery and adoption replay tests, plus the same
real PTY lifetime oracle using bare entry. `make test` includes cmd/orbit;
`make check` includes tests/terminal and `make test-terminal-packages`, which
checks every payload/checksum, actual native extracted entry, standalone repeated
installation/custom-unit preservation/uninstall, and package-extracted bare PTY.
Optional `python3 scripts/terminal_package_test.py --dist dist --containers
--emulate-arm64` performs actual Debian/Fedora manager transactions in new --rm
containers and arm64 execution through QEMU copied from the local multiarch image.
It does not establish native arm64, systemd login/logout/boot or LAN/Tailscale.

T13 adds a 1,024-file status fixture, 512 reviewed deletions with cursor paging,
and actual streamed 8/32-MiB editor merges. `make test-terminal-release` runs
these twice with both tests and spawned binaries instrumented for races. RSS and
descriptor samples describe the measured CLI processes, not an unlimited-scale
or speed guarantee. The suite also checks running-versus-startup status,
quarantined payloads/concurrent heads, and disposable-harness refusal paths.
The retirement release cases exercise exact reviewed retired-key approval refusal
and fresh replacement onboarding after retirement. Replacement must recover the
original retired-author version and bytes, including when an interrupted owner
already has approved membership without its canonical artifacts. Repository
regressions separately reject wrong artifacts and unknown retired-author history.

Release commands retain failed experiments in separate evidence directories:

```sh
python3 scripts/validation/snapshot_release.py --kernel /boot/vmlinuz-linux --output /new/reproduction
python3 scripts/validation/terminal_native.py --dist /snapshot/dist --hosts laptop rpi --output /new/lan
python3 scripts/validation/terminal_hosts.py --dist /snapshot/dist --hosts laptop rpi vps --output /new/hosts
python3 scripts/validation/terminal_private.py --dist /snapshot/dist \
  --hosts laptop rpi vps --addresses <laptop-ip> <pi-ip> <vps-ip> \
  --network tailscale --output /new/private
```

The snapshot runner commits an isolated copy of the current dirty source and
validates a clean clone, recording every input hash without changing the original
index or branch. Native runners extract checksum-verified packages into marked
roots. The LAN runner uses ordinary create/invite/join/exact approval controls;
SSH carries harness commands, while Orbit transfers directly over the recorded
interfaces. The host runner uses real PTYs and unique private systemd user units,
checks capture/restart/uninstall and preserves installed aliases. Neither runner
changes firewall/VPN policy, enables lingering or reboots an existing workload.
The private runner extends ordinary onboarding with offline-source forwarding,
exact survivor retirement review, CLI-exported membership rollout, retired-key
refusal and a fresh replacement preserving retired data. Tailscale mode requires
authenticated host addresses/routes before creating installations. A rehearsal
with fewer than three physical hosts is explicitly labeled and cannot establish
laptop/Pi/VPS acceptance.
Live SQLite inspection is replaced by authenticated history queries; interrupted
transfer waits on hash-verified immutable objects and inspects persisted progress
only after stopping its own marked worker.


W07 adds `TestWANW07RealPTYRelayOnboarding` and the keyboard runner
`scripts/terminal_wan_pty_test.py`. Run it through the Go test: that fixture
provides an independently signed local-development profile, TLS CA and marked
service-outage trigger. It requires no configured peer addresses/direct listener,
separate init/serve or host service changes. Test queries verify exact byte/hash/
version-author/head identities and persistent certificate/pin/operation/attempt;
PTY frames are supporting evidence. `ORBIT_W07_PTY_EVIDENCE=/new/empty/directory`
retains sanitized frames/results. Deliberate revealed transfer codes are excluded.
The rapid second-folder run can exhaust the historical process-local enrollment
bucket before a prepared challenge expires; successful second-folder acceptance
keeps both daemons/routes alive and allows the existing bucket to refill for 65
seconds. Restart-only attempts are retained without acceptance credit. Expired unsent transcripts
remain blocked and are never renewed by an ordinary Retry. The runner also tests
service loss, fresh missing-profile Automatic and reviewed Local-only capture.
No default hosted/native WAN, QUIC/ICE/roaming or T13 lifecycle credit is implied.


W08: `GOFLAGS=-race go test -race ./tests/terminal -run '^TestWANW08' -count=2 -v`
runs `TestWANW08BinaryLocalOnlyLANAfterApproval` (production invitation/approval,
actual daemon multicast known-peer sync, zero service requests, two-way exact heads/
hashes, persistent identities and outage capture) and
`TestWANW08BinaryOptionalCollisionFreshRelayOnboarding` (fresh real CLI setup with
optional ports occupied, verified relay bytes and mandatory manual-listener failure).
Both use marked disposable roots; five-second production reconciliation is retained.
Restart-to-relay quota/generation stress is uncredited and remains W11.
Public-scope IPv4/IPv6 namespace fixtures live in `internal/replication` and require
the separate marked `scripts/wan_direct_namespace_test.py` runner; they are simulated,
while the ordinary test package reports actual local IPv6 availability.

### W11 roaming and measured peer fairness

`TestWANW11BinaryReviewedTimingAndRestart` runs the actual CLI/daemon policy review,
apply/replay, restart and default restoration. `TestWANW11ActualLargeSmallPeerBandwidthAcrossRoutes`
enrolls three trusted replicas, transfers a real 16 MiB version concurrently with
continuous small versions over WSS, TCP and HTTP3, and reports shared rate, combined
sampled heap/FD/goroutine peaks and CPU. Discover these names before executing;
ordinary aggregate execution skips the namespace-only whole-daemon case.

Compile `go test -c -race -o /tmp/orbit-w11-terminal.test ./tests/terminal` and run
`scripts/wan_daemon_roaming_namespace_test.py` under a newly created
`unshare --user --map-root-user --net` namespace. Supply the compiled `--test-binary`,
a canonical private `--root` containing a regular `.orbit-disposable` marker,
and the host network namespace identity as `--parent-namespace` (captured before
unshare). The runner validates the owning user namespace before any `ip`, firewall
or `tc` action. `--latency-ms 25 --loss-percent 1` impairs actual peer/service
traffic while exempting loopback owner control; `--default-timing` uses production
poll/reprobe/cooldown defaults. Otherwise the fixture applies reviewed faster
poll/reprobe settings without altering identity or service quotas. No existing
Pi/VPS/host network namespace may be used for these mutations.

For Pi measurements, cross-compile the test binary with
`CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go test -c ...`, copy it and the runner into
a newly marked private remote root, and invoke the same guarded namespace runner.
The Pi need not install Go. The daemon sampler records process metrics beneath
the disposable root; capture these before cleanup. This tests the production
`app.ServeWithOptions` lifecycle in child test binaries, rather than an installed
package/systemd/login session. Native host lifecycle remains T13.
