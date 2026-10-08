# W15 disposable WAN failure campaign

Run from the repository root on Linux with Go, Python 3, `unshare`, `ip`, `tc`,
`iptables` and `ip6tables` installed. The kernel must permit remapped user and
network namespaces. No host firewall or sysctl change is needed. Missing tools
or namespace permission leave that campaign unexecuted. Do not run against an
existing service, personal sync root or shared VPS.

The worker accepts a fresh private `/tmp/orbit-w15-*` directory with the exact
single-link, owner-only disposable marker below. It refuses root aliases,
symlinks, `..`, other prefixes/locations, personal-pilot markers and prior run
contents. Before every network command it checks that the network namespace
differs from its parent, belongs to the current user namespace, and has exactly
one remapped UID. These checks use exceptions, so Python `-O` cannot remove them.
An exit-zero test executable with zero matched, skipped or missing required
tests is refused as unexecuted coverage. Timeout signalling accepts only its still-running direct child with the exact
copied executable and namespace. Daemon crash fixtures additionally validate
marked state, the direct child process handle and its recorded daemon PID; W15
children die if their test parent exits. There is no arbitrary PID or remote
endpoint argument.

Compile and discover first:

```sh
go test -list '^TestWANW15' ./internal/... ./tests/terminal
go test -c -race -o /tmp/orbit-w15-terminal.test ./tests/terminal
go test -c -race -o /tmp/orbit-w15-replication.test ./internal/replication
python3 -m unittest discover -s scripts/validation -p test_wan_safety.py
```

Allocate a **new root for each invocation**, then enter a new namespace:

```sh
run_root=$(mktemp -d /tmp/orbit-w15-XXXXXXXX)
chmod 700 "$run_root"
printf 'orbit W15 disposable network/process campaign\n' > "$run_root/.orbit-disposable"
chmod 600 "$run_root/.orbit-disposable"
parent_netns=$(readlink /proc/self/ns/net)
unshare --user --map-root-user --net python3 scripts/wan_failure_campaign.py \
  --root "$run_root" --parent-namespace "$parent_netns" \
  --test-binary /tmp/orbit-w15-terminal.test --suite crash --scenario impaired
```

The suite choices are fixed, with no caller-selected command or PID:

| Suite | Binary | Executed fixture and oracle |
| --- | --- | --- |
| `crash` | terminal | W15 whole production daemons: QUIC chunk/receipt SIGKILL, relay recovery, interface/default-route change, blocked peer TCP/UDP, service outage/restart; exact heads/authors/manifests/bytes/identity/pin and large-version endpoint receipt |
| `fairness` | terminal | W11 actual three-peer 16 MiB plus continuous small versions on WSS/TCP/HTTP3, shared 2 MiB/s limiter; verified large/small progress, CPU/RSS/heap/FD/socket/goroutine samples |
| `enrollment` | terminal | W05 actual daemon SIGKILL at prepared/accepted/approved/membership-received boundaries; exact durable operation/attempt/request/root, identity, two-way hashes and no duplicate registration |
| `ice` | replication | W10 actual Pion/HTTP3, failed gather with WSS fallback, two directions, interrupted verified-chunk reuse, handler/pin/body isolation and UDP6 STUN |
| `public` | replication | W08 actual authenticated directory and TCP over simulated public IPv4/IPv6; exact bytes and pins |

`recovery` adds no impairment; `impaired` applies kernel netem 25±5 ms delay,
1% loss, 5% reordering with 50% correlation, 20 Mbit/s rate and a 1,000-packet
queue. Loopback owner-control addresses are exempt; native peer/service traffic
uses addresses on the disposable dummy interface and passes through the impaired
loopback path. `mtu` sets both disposable interfaces to 1,280 bytes. None of
these addresses are routed to the public internet. Full commands, namespace,
scenario, exit code and elapsed time remain in `network.json`, `result.json`
and `campaign.log`. Daemon metrics are sampled every 250 ms in JSONL files.
The whole-daemon log names a five-second idle interval for CPU/resource analysis.

The in-process NAT emulator supplies independent/dependent filtering/mapping,
double NAT and blocked UDP configurations through real Pion/HTTP3 plus native
HTTPS/WSS service composition. It requires no privileges:

```sh
go test -race -count=1 -v ./internal/network ./internal/replication \
  -run '^TestWANW10(ICEHTTP3NATMatrix|IncompatibleDoubleNAT|AuthenticatedNATPeerMatrix)$'
go test -race -count=1 -v ./internal/replication ./internal/testkit -run '^TestWANW15'
```

Adversarial admission, directory substitution/replay/expiry/SSRF, unknown
endpoint/purpose/folder, relay ciphertext/frame/quota/flood, malformed STUN,
slow readers and cancellation are existing W01–W14 production regression
fixtures, enumerated by `go test -list '^TestWANW' ./internal/...`. W15 executes
them in the uncached full race campaign and records a scenario matrix. Network
failures do not change the owning membership/publication/GC contracts. W15 adds
an independent transfer-boundary model, aggressive protected-content GC,
post-rename publication recovery, conflict-preserving route changes and cached
transport refusal after retirement. Failures log replayable boundary traces.

Fuzz each target separately, preserving any minimized corpus on failure:

```sh
go test ./internal/protocol -run '^$' -fuzz '^FuzzWANW15NetworkCodec$' -fuzztime=30s -parallel=4
go test ./internal/protocol -run '^$' -fuzz '^FuzzWANW15SignedProfile$' -fuzztime=30s -parallel=4
go test ./tests/faults -run '^$' -fuzz '^FuzzProtocolEnvelopeDecode$' -fuzztime=30s -parallel=4
go test ./tests/faults -run '^$' -fuzz '^FuzzPathSanitization$' -fuzztime=30s -parallel=4
```

Ordinary checks never invoke privileged network mutations. Release campaigns
add `make check`, `go test -race -count=1 -timeout=35m -v ./...`, `make demo`
and client/operator packages. Go skip messages remain unexecuted coverage;
separate guarded campaigns supply evidence for their specific cases.

Copy sanitized topology/transcripts/metrics before cleanup. This worker retains
its allocated root for inspection and never recursively deletes a caller path.
A manual cleanup must revalidate the marker/canonical/private/owned root and
prove no fixture process uses it. SIGKILL establishes process recovery only;
it does not establish power-loss durability. Model/emulator/native socket
campaigns do not prove physical WAN connectivity, WG6 operator readiness,
N10 hosted-default acceptance, T13 login/logout/boot checks or owner use.
