# W16 native WAN runner

Run from the repository root with Linux, Python 3, OpenSSL, `ip`, SSH and the
current amd64/arm64 Orbit archives plus `SHA256SUMS` and `release-manifest.json`.
The worker extracts the checksummed executable in memory; no archive paths are
installed on the host. SSH aliases must already be configured for batch access.

The owner's laptop/Pi behind the home router and the Oracle VPS are two physical
networks. A public address on every replica is unnecessary: relay connections
are outbound, and ICE can attempt supported traversal. Two home devices talking
through a remote broker alone do not satisfy the separate-network replica case.
Record actual public/direct/relay routes before granting native acceptance.

This first runner slice provides read-only topology inventory and a packaged CLI
journey: fresh create/invite/join, exact identity/pin approval, two-way small files,
a deterministic 4 MiB version, capture while a receiver is stopped, restart,
second-folder enrollment, protected hashes/heads and endpoint stored receipts.
It does **not** certify the full W16 packet. Physical TUI, forced UDP-blocked relay,
address/interface change, disposable service restart, three-host forwarding,
conflict/restore and complete native resource/route instrumentation remain required.

## Read-only preflight

Create a topology file with the actual deployment facts (labels are declarations,
not automatic proof of physical separation):

```json
{
  "hosts": [
    {"host": "laptop", "role": "Laptop", "physical_network": "owner-home-router"},
    {"host": "vps", "role": "VPS", "physical_network": "Oracle-ca-toronto-1"}
  ]
}
```

```sh
python3 scripts/validation/wan_native.py \
  --topology /path/to/topology.json --route-targets 132.145.111.200 \
  --inventory-only --output /path/to/new-empty-preflight-directory
```

Inventory requires no marker or WG6 closure because it only reads interfaces,
addresses, route lookup and policy rules. It never starts Orbit or contacts the
service. Numeric route targets are observations, not manual Orbit endpoints.
Report `success` means the requested inventory was collected. Its per-host
`route_preflight` reports limitations; acceptance fields stay `unexecuted`.

Ordinary hosted execution reads the authoritative WG6 outcome in
[`docs/orbit-wan-design-gates.md`](../../docs/orbit-wan-design-gates.md). There is
no gate-override flag. While it is open the runner fails before any host action.
After closure, omit `--inventory-only` to run the CLI journey:

```sh
python3 scripts/validation/wan_native.py --dist dist \
  --topology /path/to/topology.json --route-targets 132.145.111.200 \
  --output /path/to/new-empty-journey-directory
```

The preflight conservatively refuses any active WireGuard, Tailscale, TUN/TAP or
other recognized tunnel, missing/ambiguous route or nonphysical uplink. It does
not turn off VPNs or modify host routing. The current hosts have active tunnels;
a dedicated isolated replica environment and its actual route evidence are still
needed. Passing this preflight alone cannot prove the full traffic path: final
packet acceptance additionally needs observed routes and socket/packet evidence
during each transfer. The report therefore keeps physical/hosted acceptance
`unexecuted` even after a successful journey; assess those independently.

No `SSHRelay`, port-forwarding, VPN fallback, manual endpoint/listener settings,
prebuilt membership or copied device identity is used. SSH runs with
`ClearAllForwardings=yes`, `ControlMaster=no`, `ControlPath=none` and no forwarding
arguments. It may administer hosts via existing private access; that connection
does not carry the Orbit route. Worker children remove inherited HTTP/SOCKS proxy
variables and set the packaged-profile switch explicitly. Hosted mode checks
the archive checksum, binary's embedded profile, package manifest, reviewed
Automatic policy and system TLS trust.

## Isolated replica namespaces

Hosts with active Tailscale/WireGuard run replicas inside a namespace created by
[`wan_netns.sh`](wan_netns.sh) (needs root on that host and an explicit
`ORBIT_W16_DISPOSABLE=1`). Its only link is a veth NATed to the physical uplink;
host FORWARD rules reject every other egress interface, so no tunnel candidate
exists or can be routed. Add `"netns": "orbit-NAME"` to that host's topology
record. The worker then runs as the SSH user via
`sudo -n ip netns exec orbit-NAME sudo -n -H -u <user>`, and the preflight
requires exactly one veth inside plus the tagged confining rules toward one
physical uplink on the host. Each transfer records the host NAT/forward counter
deltas (`route_counters`) as route evidence. `wan_netns.sh down` removes exactly
the tagged rules and the namespace; rules are runtime-only.

### Owner-started namespace (no runner sudo)

A host where the runner has no passwordless sudo (the laptop) joins as the
topology's `"third"` record with `"shell": "/home/USER/orbit-w16/run/shell.sock"`.
Copy `wan_netns.sh`, [`wan_owner_netns.sh`](wan_owner_netns.sh) and
[`wan_netns_shell.py`](wan_netns_shell.py) to `~/orbit-w16/`; the owner runs once
`sudo ORBIT_W16_DISPOSABLE=1 ~/orbit-w16/wan_owner_netns.sh up USER UPLINK OCTET`.
That saves and enables `ip_forward` (runtime only), creates the namespace, saves
the tagged rules to `~/orbit-w16/netns-snapshot.txt` and starts a user-owned
socket shell inside the namespace (exits after 4 h idle). Worker calls then go
through that socket as the same user; preflight parses the snapshot with the same
confinement checks. Path accounting is not available there (it needs root); the
runner records the namespace interface totals instead. The same command with
`down` stops the shell, removes the namespace and restores `ip_forward`.

## Journeys and drills

- `--journey tui` replaces CLI create/invite/join/approve with keyboard phases on
  real PTYs ([`wan_tui_phase.py`](wan_tui_phase.py)); redacted frames go to the report.
- `--impairments` blocks UDP in the second host's namespace (forced relay), then
  changes the first host's namespace address.
- `--service-restart SSH_ALIAS` restarts `orbit-net` once during forced relay and
  requires relay recovery plus a transfer each way. **Owner approval is required
  for every use against a production service.**
- `--three-host` adds the third host: join, then forwarding (third device offline
  while the second reconnects), a two-head conflict resolved on the third device,
  and a restore on the second device.
- `--service-metrics-host` reads the service's loopback counters and `/proc`
  sample (read-only). Every run samples daemon RSS/peak/CPU/fds/state size.

## Safe local rehearsal and refusal tests

The production-binary fixture creates a local private CA and a disposable
self-hosted service and feeds a synthetic checksummed archive to the same runner.
It sets no listener addresses on the replicas. It does not contact the hosted
service, does not use physical WAN and does not represent a release archive.

```sh
python3 -O -m unittest discover -s scripts/validation -p test_wan_native.py -v
go test -list '^TestWANW16' ./tests/terminal
GOFLAGS=-race go test -race -count=1 -v -timeout=12m ./tests/terminal \
  -run '^TestWANW16NativeRunnerRehearsal$'
```

An optional `ORBIT_W16_REHEARSAL_REPORT=/absolute/new/report.json` saves the
sanitized rehearsal result exclusively; an existing file is refused. Only
local hosts can use `--rehearsal-profile` with `--rehearsal-roots`; these switches
explicitly label the run as a self-host rehearsal with native and bundled-default
acceptance unexecuted. This is a compatibility/development fixture, never a
WG6 exception.

The worker allocates new private `~/orbit-validation-*` roots and exact random
owner-only disposable markers. Reuse, pilot markers, symlink escapes, unsafe
markers and unrelated PIDs are refused. Process stopping reuses the exact
executable/state/start-time guard, allowing 15 seconds for graceful shutdown.
Every CLI/SSH call has a finite deadline. On success only the fresh marked roots
are removed, after owned daemon shutdown and a final live-process reference check.
On failure, roots/private CLI output are retained and cleanup errors revoke run
success. No caller-selected root, PID, firewall or route action is exposed; the only
service action is the explicit `--service-restart` drill.

Invitation codes are transferred through private files and never appear in
argv, public errors or reports. Identity checks extract only the public
certificate locally and hash its SPKI using OpenSSL; private PEM is never passed
to a subprocess or returned to the controller. Byte/version/author comparisons
and stored receipts identify the receiving device, not the broker. Remote
`applied=false` remains unchanged in the record; actual receiver working hashes
and qualified readiness are observed separately. Reconnect preserves the
already-captured version and retries preserve the reviewed operation/attempt.

Use W15's separate guarded campaigns for isolated emulator faults. Restart the
deployed VPS service only with the owner's explicit approval for that run, and
never change an existing workload's network policy to obtain W16 fault evidence. School/corporate/CGNAT/IPv6 access must have actual
deployment evidence before claims.
