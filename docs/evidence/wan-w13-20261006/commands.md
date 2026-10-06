# W13 validation commands

The tree already held uncommitted W00–W12 work; nothing unrelated was reset.
No pre-edit hash snapshot was taken for this packet; `check_docs.py` records
final hashes of every W13-owned file in [`final-source.json`](final-source.json)
against the current `HEAD` revision.

## Format, vet and packages

| Command | Result |
| --- | --- |
| `make fmt-check` | Passed (after formatting `tests/terminal/wan_w13_test.go`) |
| `go vet ./...` | Passed |
| `go test -count=1 ./cmd/... ./internal/... ./model/...` | Passed; no failing package |
| `go test -race -count=1 ./cmd/orbit-net/ ./internal/rendezvous/ ./internal/network/ ./internal/config/ ./internal/control/... ./internal/app/ ./cmd/filesync/` | Passed ([log](logs/race-packages.log)) |
| Focused W13 unit, inspection and rotation tests (see log) | Passed ([log](logs/inspection-rotation.log)): `TestWANW04BrokerSeesOnlyInnerTLSCiphertext`, `TestWANW01PinnedTransport`, `TestWANW13RotationOverlapAndSanitizedMetrics`, three `cmd/orbit-net` tooling tests and two config tests |

Earlier failures during development: an initial
`TestWANW04PerPairAdmissionCannotBeBypassedByNewClient` and
`TestWANW04ServiceCapacityResourcesAndEnrollmentIsolation` failure came from
fixtures injecting sessions without a profile digest, which production never
creates; both fixtures now carry the served digest. The first rotation-test
assertion expected stale challenges that correctly expired after 60 seconds.

## Real-binary rehearsal

```text
go test -count=1 -timeout 15m -v ./tests/terminal -run '^TestWANW13'
make package-orbit-net
ORBIT_NET_ARCHIVE=$PWD/dist/orbit-net-v1.0.0-linux-amd64.tar.gz \
  go test -count=1 -timeout 15m -v ./tests/terminal -run '^TestWANW13'
```

Both pass: fresh build inside the suite run below (122 s); packaged archive in
176.97 s ([packaged log](logs/rehearsal-packaged.log)). One earlier run failed
because the test pointed `SSL_CERT_DIR` at the directory holding the CA, so the
"no reviewed trust" device trusted it through system roots; the test now uses an
empty directory. Measured in the packaged run: see the log's `overload:` and
`STUN:` lines.

Negative reproduction of the rotation defect: temporarily restoring strict
`digest == c.digest` in `ServiceClient.servedProfile` makes the same rehearsal
fail at `mixed-epoch.txt` ("verified bytes did not arrive", 193 s); the source
was restored and the hash recorded in `final-source.json`.

## Packaging and platforms

| Command | Result |
| --- | --- |
| `make package-orbit-net` twice; `diff` of `dist/orbit-net-SHA256SUMS` | Identical: reproducible ([sums](orbit-net-SHA256SUMS)) |
| `tar -tzvf dist/orbit-net-v1.0.0-linux-arm64.tar.gz`; `file bin/orbit-net-linux-*` | Expected layout; static amd64 and aarch64 ELF |
| `systemd-analyze verify packaging/systemd/orbit-net.service` (copied) | Only reports the binary absent from `/usr/bin` on this host |
| [`w13-systemd.sh`](w13-systemd.sh) with the amd64 archive | Passed ([log](logs/systemd-sandbox.log)): `--check`, serve, healthz, TLS, SIGHUP reload and stop under `@system-service` seccomp, MemoryDenyWriteExecute, address-family, namespace, memory, task and FD limits as a `systemd-run --user` unit; result `success` |
| `ssh rpi` + [`w13-pi-smoke.sh`](w13-pi-smoke.sh) with the arm64 archive | Passed ([log](logs/pi-arm64-smoke.log)) on the owner's Pi 4B in a marked `/tmp/orbit-w13-smoke-*` directory, removed afterwards; no existing service, folder, firewall or sysctl touched |

## Affected WAN suites

```text
go test -count=1 -timeout 45m -v ./tests/terminal -run '^(TestWANW0[567]|TestWANW1[0123])'
```

Result: **pass**, exit 0 in 792.361 s ([terminal-wan.log](terminal-wan.log)):
W05 routed join/restart/third-device, W06 reviewed relay/local/guided, W07 PTY
onboarding, W11 bandwidth and timing, W12 doctor/privacy/PTY and W13 rehearsal.
`TestWANW05DaemonChild` (child helper) and `TestWANW11WholeDaemonRoamingAndMixedProgress`
(isolated-namespace only) skip by their guards and earn no credit here.
`TestWANW08BinaryOptionalCollisionFreshRelayOnboarding` was excluded; it remains
the recorded W12 aggregate timeout and was not re-run.

## Documentation

```text
python3 docs/evidence/wan-w13-20261006/check_docs.py
git diff --check
```

Result: **pass** — 185 local links/anchors valid, 30 final source hashes
recorded; no whitespace errors.

## Unexecuted

- Root-only unit properties (`User=orbit-net`, `AmbientCapabilities`,
  `ProtectSystem=strict`, `ConfigurationDirectory`, `ProtectHome`) and the
  sysusers entry on a real system manager.
- Any public deployment: no operator, DNS name, public host or publicly trusted
  certificate exists. Hosted capacity and egress are not measured.
- arm64 real-binary rehearsal with device daemons (no Go toolchain on the Pi; the
  smoke covers the service binary only).
- Full `go test ./...` aggregate.
