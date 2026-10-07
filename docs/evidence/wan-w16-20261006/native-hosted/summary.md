# W16 native hosted-default runs — 2026-10-07

State: **complete for the reachable W16 cases.** The defect found in runs 3–4 is
fixed ([fix evidence](../quota-fix/summary.md)). Runs 19–20 add the keyboard TUI
journey, the owner-approved service restart, the laptop as a third host and
resource samples. School/corporate/CGNAT/IPv6 remain unexecuted (no access).

Topology ([topology-netns.json](../topology-netns.json)): Pi on the owner's home
network and the Oracle VPS, each replica inside a `wan_netns.sh` namespace whose
only exit is NAT to the physical uplink (`eth0` / `enp0s6`). No Tailscale,
WireGuard or SSH forwarding can carry Orbit traffic. Packages: fresh `make
package` from the working tree ([log](package.log)); `orbit-v1.0.0-linux-arm64.tar.gz`
sha256 `ebe62d0e…74a`, bundled profile digest `356f0ced…c165` (epoch 1, expires
2027-01-04), system TLS trust, Automatic policy. The release set was staged
without the operator archives because the runner's package glob also matched
`orbit-net-*`.

Each namespace counts bytes per path with namespace-local rules: TCP/8443 to
the service = relay/rendezvous, UDP/3478 = STUN, other traffic at the service
address = the co-located VPS replica (direct), anything else = direct peer.
Only received bytes count as a path. Loopback (worker ↔ daemon) is excluded.

## Results

| Run | Command | Exit | Notes |
| --- | --- | --- | --- |
| [run1](run1/wan-native.json) | `wan_native.py --dist <staged> --topology topology-netns.json --route-targets 132.145.111.200` | 0 | full CLI journey in 126 s; host NAT counters only (cannot separate relay from direct because the relay is also on the VPS) |
| [run2](run2/wan-native.json) | same + `--impairments` | 1 | accounting bug: loopback and co-located UDP misclassified; uncredited |
| [run3](run3/wan-native.json) | same + `--impairments` | 1 | journey passes; reverse transfer under UDP block times out |
| [run4](run4/wan-native.json) | same + `--impairments` | 1 | reproduces run3 with diagnostics |

Run3 (and run4) journey observations:
- Join to approved: ordinary create/invite/join/approve with no addresses or profile files.
- **Real direct cross-network sync:** the logical 4 MiB version arrived over
  direct UDP, Pi → VPS: the VPS received 1,106,298 direct-UDP bytes against 4,464
  service-TCP bytes (chunk deduplication shrinks the 4 MiB pattern to about 1.1 MB).
  First small file was also direct-dominant.
- **Real relay cross-network sync:** the reconnect, second-folder and second
  small-file transfers were relay-dominant (for example reconnect 98,406 relay
  bytes vs 1,606 direct). Forced relay with UDP blocked in the VPS namespace (run4): routes
  became `relay` on both sides in 7.4 s and a 1 MB file Pi → VPS arrived in 11.4 s with
  **zero** direct bytes and 1.16 MB over the service.
- Receipts identify the receiving device (`stored: true`; `applied` stays
  `false` as reported), protected hashes/heads agree and identities/pins stay
  stable, as in the earlier rehearsal oracles.

## Defect found (now fixed; original record retained)

With UDP still blocked on the VPS, the reverse transfer VPS → Pi never reached
the Pi in 180 s (runs 3 and 4): the Pi's history stayed empty, neither side had
a connected peer route at timeout, and about 2 MB of service TCP churned each
way. The VPS daemon logged repeated `TLS handshake error from orbit-relay:0:
i/o timeout` ([log](run4-vps-daemon.log)). Normal relay VPS → Pi works (run3
second folder), so the failure is in recovery after the direct UDP path is lost
mid-session. The Pi daemon's output was not captured. The address-change drill
did not run because it comes after this step.

## Not executed

Keyboard TUI journey; address/interface change (blocked by the defect above);
relay/rendezvous restart (no disposable public service); laptop third host and
forwarding/conflict/restore; resource sampling; school/corporate/CGNAT/IPv6.

## Fix iterations and final runs (2026-10-07)

Runs 5–13 and two diagnostic series drove the fix; each retained report keeps its
own result. Run 5 (timeline) showed `QUOTA_EXCEEDED` for ~80 s; run 6 (first
client fix) completed the UDP-blocked reverse in 12.7 s but failed on a
loop-back multicast accounting bug (fixed: LAN discovery is now its own class);
run 7 failed (Pi starved by accepting offers); runs 8–12 passed with tails of
54.7 s (forced relay) and 80.8/115.2 s (address change / reverse) that led to
whole-setup admission, control rebuild and the one-tunnel-per-direction fix.
**Run 13 overlapped a diagnostic run on the same namespaces and is uncredited.**
Diagnostic builds (`W16DIAG` logging) were temporary and never committed or
published; their outputs stayed in scratch.

Final source, fresh `make package` (`orbit-v1.0.0-linux-arm64.tar.gz` sha256
`d0c2b1ec…8689`), runner restarts using the product default interval,
`wan_native.py --impairments --service-metrics-host vps`, three consecutive runs:

| Run | Join→approved | 4 MiB direct | UDP-blocked Pi→VPS 1 MB | UDP-blocked VPS→Pi | Address change: first transfer / reverse | Service quota refusals during forced relay |
| --- | --- | --- | --- | --- | --- | --- |
| [14](run14/wan-native.json) | 31.4 s | 6.5 s, 1.12 MB direct UDP | 5.5 s, 0 direct bytes | 5.5 s | 30.9 s / 6.5 s | 0 (4 during address change) |
| [15](run15/wan-native.json) | 33.6 s | 6.1 s | 5.9 s, 0 direct bytes | 5.4 s | 32.7 s / 5.7 s | 0 (0 during address change) |
| [16](run16/wan-native.json) | 31.3 s | 6.2 s | 6.8 s, 0 direct bytes | 5.5 s | 30.8 s / 6.4 s | 0 (4 during address change) |

All three: success, no cleanup errors, no retained roots; receipts identify the
receiving device; hashes/heads agree. Address-change recovery (~31 s) is bounded
by the service holding the old control channel until its heartbeat drops it; a
service-side replacement would shorten it but needs a production redeployment.
Single-run measurements on one home network and one VPS; not universal deadlines.

### Final-source confirmation (after the diagnostic-probe slot bypass)

`make check` failed twice during the last edits and was fixed before acceptance:
first `TestWANW12BinaryDoctorPrivacyAndPTY` (the relay probe waited behind the
pool's tunnel slot; the explicit probe now bypasses the slot), then
`TestWANW10AuthenticatedNATPeerMatrix` (an extra deadline fast-fail in budget
waits; reverted). Final `make check` passes
([log](../quota-fix/make-check-3.log); earlier failing logs retained). Fresh
packages (`orbit-v1.0.0-linux-arm64.tar.gz` sha256 `596ceece…18fb`):

| Run | Join→approved | 4 MiB direct | UDP-blocked Pi→VPS 1 MB | UDP-blocked VPS→Pi | Address change first / reverse | Service quota refusals during forced relay |
| --- | --- | --- | --- | --- | --- | --- |
| [17](run17/wan-native.json) | 35.3 s | 6.1 s | 7.8 s, 0 direct bytes | 5.3 s | 21.5 s / 8.0 s | 0 (4 during address change) |
| [18](run18/wan-native.json) | 31.3 s | 6.2 s | 5.0 s, 0 direct bytes | 5.7 s | 28.1 s / 4.3 s | 0 (2 during address change) |

## TUI, service restart, three hosts and resources (2026-10-07)

Same packages as runs 17–18 (no product source changed; `orbit-v1.0.0-linux-arm64.tar.gz`
sha256 `596ceece…18fb`, amd64 `fa24501b…7546` for the laptop). Runner additions:
`--journey tui`, `--service-restart`, `--three-host`, per-stage resource samples
([guide](../../../../scripts/validation/WAN_NATIVE.md#journeys-and-drills)). The local
rehearsal passes both variants ([log](rehearsal-tui-three-host.log): CLI 107 s,
TUI + three hosts 196 s); 36 harness unit tests pass.

**[Run 19](run19/wan-native.json)** — `wan_native.py --dist <staged> --topology topology-netns.json
--route-targets 132.145.111.200 --journey tui --impairments --service-metrics-host vps
--service-restart vps` → exit 0, 268 s ([log](run19.log)).

- Keyboard TUI on real PTYs, Pi (owner) and VPS: create reviewed the packaged
  profile operator; the saved invitation is routed v3 with no endpoints; join
  reviewed the same operator; approval showed the same verification code as the
  joiner. Join → completed 31.2 s. Terminal state restored after every phase; the
  joiner's connection screen showed "Direct connection". Redacted frames are in
  the report (`scenarios.tui.frames`).
- Then the full journey: two-way small files, 4 MiB in 8.4 s over direct UDP
  (1.13 MB direct vs 16 KB service), reconnect, second folder, forced relay
  (1 MB Pi → VPS 6.8 s and reverse 6.5 s, zero direct bytes), address change
  (first transfer 40.0 s, reverse 6.5 s).
- **Service restart** (owner-approved, once): `systemctl restart orbit-net` on the
  VPS while UDP stayed blocked. Active again after 0.3 s (new PID, same binary and
  config); both devices ready with relay routes 9.5 s after the restart; transfers
  afterwards 11.7 s Pi → VPS and 7.7 s VPS → Pi, zero direct bytes. The
  alert timer kept passing (Down needs two failed samples a minute apart) and
  sent nothing. The report's `active_enter_monotonic_us`/`restarts` service fields
  were mis-parsed in this run (fixed afterwards); the PID change is correct.

**[Run 20](run20/wan-native.json)** — `wan_native.py --dist <staged> --topology
../topology-netns-3.json --route-targets 132.145.111.200 --three-host
--service-metrics-host vps` → exit 0, 283 s ([log](run20.log)). The laptop ran
inside an owner-started namespace NATed to Wi-Fi `wlp2s0` (preflight: only its
veth inside, route via the veth, confining rules in the saved snapshot).

- CLI journey as before, then the laptop joined (approved 35.1 s) and its file
  reached both others (10.0 s).
- **Forwarding:** VPS stopped, laptop authored, Pi received; laptop stopped, VPS
  restarted and received the laptop-authored version from the Pi in 7.8 s
  (author unchanged; the laptop was offline throughout delivery).
- **Conflict:** VPS edited while stopped, laptop edited online; after the VPS
  restarted, all three showed the same two heads within 13.7 s. Resolved on the
  laptop by selecting the VPS version; bytes and cleared attention on all three
  in 8.1 s.
- **Restore:** the VPS restored the shared base; all three had the base bytes
  under one new VPS-authored version identity within 10.2 s.

Laptop path accounting is limited to namespace interface totals (no root for
per-path rules). Laptop and Pi share the home network; this case establishes
three-device engine behaviour over the hosted WAN service, not a third network.

### Resources (run 19 unless noted; `/proc` samples)

| Stage | Pi daemon RSS / peak | VPS daemon RSS / peak | Laptop daemon RSS | orbit-net RSS |
| --- | --- | --- | --- | --- |
| after join | 30.5 MiB | 29.8 MiB | — | 15.1 MiB |
| after 4 MiB | 34.2 MiB | 34.5 MiB | — | 15.2 MiB |
| after second folder | 32.0 / 34.0 MiB | 31.9 MiB | — | 14.9 MiB |
| after impairments + restart | 31.9 / 34.0 MiB | 32.0 / 33.9 MiB | — | 14.2 MiB (new process) |
| run 20 after three-host | 32.8 MiB | 30.9 MiB | 35.7 MiB | 14.8 MiB |

CPU time over run 19: Pi daemon 9.9 s user + 2.1 s system, VPS daemon 2.0 + 0.7 s;
21–25 open fds and 11–12 threads per daemon; state directories 6.7–6.8 MB. Single
runs on these hosts, not limits.

### Cleanup (2026-10-07)

All disposable `filesync-validation-*` roots were removed after a live-process
check (33 local, 33 Pi, 8 VPS; none on the laptop). Owner pilot roots
`FileSyncPilot-20261001` and their daemons were left untouched. The Pi and VPS
namespaces were removed with `wan_netns.sh down` (no tagged rules remain); the
laptop namespace is removed by the owner's `wan_owner_netns.sh down`. Production
`orbit-net` and the alert timer remain active.

Final `make check` on the closeout source passes ([log](make-check-closeout.log)); the terminal suite took 1,502 s of its 30 min limit, so W17 should watch that margin.
