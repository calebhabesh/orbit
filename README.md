# Orbit: Personal File Manager & Decentralized Synchronization

Orbit is a personal file manager and peer-to-peer synchronization engine over trusted Linux replicas. A single static Go binary watches configured workspace roots, retains historical versions in an immutable content-addressed store, and synchronizes files over authenticated mutual TLS. SQLite stores causal DAG history, working-copy state, transfer progress, and durable recovery journals.

Independent offline edits remain distinct heads until you explicitly review and resolve them. Replicas hold readable files on disk. Pure-Go SQLite (`modernc.org/sqlite`) and an embedded React/TypeScript web management console require **zero external C compiler, libc coupling, or Node.js runtime**.

For legacy installations, full compatibility with the existing `filesync` engine commands, service units, and state layouts is strictly preserved (Gate G05).

---

## Key Guarantees and Architecture

- **Causal Consistency (Invariants I01–I06)**: File versions form a directed acyclic graph (DAG) stamped with monotonic author counters. Independent concurrent edits never silently overwrite each other.
- **Durable File Mutations & Recovery Preservation (Gate G03, Invariant I26)**: File creation, import, move, and recursive deletion record phase transitions in SQLite journals. Destination overwrites displace previous contents into `.filesync-internal/recovery/` rather than unlinking bytes. Concurrent source modifications preserve both copies.
- **One-Use Bootstrap Security (Gate G01, Invariant I21)**: Launching Orbit issues a short-lived (60s TTL), high-entropy bootstrap token via URL fragment. Loopback Host and Origin validation prevent DNS-rebinding attacks. Browser logout terminates control sessions without interrupting background daemon sync.
- **Explicit Membership & Fork Detection (Gate G02, Invariant I24)**: Pairing requires mutual Ed25519 key possession proof and explicit owner review. Linear membership rollout (`Revision N+1`) detects competing partitioned approvals and prevents untrusted device admission.
- **Stopped Metadata Restore & Counter Monotonicity (Gate G04, Invariant I08)**: Restoring older metadata backups enforces an exclusive stopped daemon lock, safely re-keying the node identity to guarantee author counters never roll backward.
- **Storage Accounting & Bounded Pruning (Invariants I10, I28)**: Reports byte usage distinctly across 5 categories (working-root, managed CAS chunks, staging, recovery, database). Retention inspection reads never trigger deletion; garbage collection remains an explicit mutation. Completed lifecycle records are safely pruned without growing SQLite indefinitely.

---

## Build and Package

Orbit builds with the Go toolchain (1.27.1), Make, and Linux. Embedded web assets are pre-built in `web/dist`.

```sh
# Build native and cross-architecture binaries (bin/filesync and bin/orbit symlink)
make build build-arm64

# Inspect build metadata, schema version, and embedded asset digest
./bin/orbit version
./bin/orbit version --json

# Run local demo with loopback peers
make demo

# Run complete test verification (unit, integration, model, fault boundaries)
make check
make test-race

# Build release packages (.tar.gz, .deb, .rpm) and cryptographic SHA256SUMS
make package
```

Release packages generated in `dist/` include:
- `orbit-v1.0.0-linux-amd64.tar.gz` / `filesync-v1.0.0-linux-amd64.tar.gz`
- `orbit-v1.0.0-linux-arm64.tar.gz` / `filesync-v1.0.0-linux-arm64.tar.gz`
- `filesync_1.0.0_amd64.deb` / `filesync_1.0.0_arm64.deb`
- `filesync-1.0.0-1.x86_64.rpm` / `filesync-1.0.0-1.aarch64.rpm`
- `release-manifest.json` and `SHA256SUMS`

---

## Getting Started

### 1. Launching Orbit on Desktop

Launch the daemon and open the web management interface in your default browser:

```sh
orbit launch
```
*(Or simply execute `orbit` without arguments, or launch "Orbit" from your desktop application menu).*

The launcher automatically detects existing running daemons via exclusive lock, generates a secure one-use bootstrap handoff URL (`http://127.0.0.1:<port>/#bootstrap=<token>`), and opens your browser.

### 2. Headless and Server Setup

On headless servers, Raspberry Pis, or cloud VPS instances:

```sh
# Check operational status
orbit status

# Configure workspace root and initial setup
orbit setup --root /srv/orbit/notes --label "Backup VPS"

# Enable user session lingering (CRITICAL: keeps background sync active across logout)
loginctl enable-linger $USER

# Enable and start the background sync user service
orbit service enable
orbit service start
```

### 3. Pairing Devices

```sh
# On primary workstation (Host): create pairing invitation
orbit invite create --ttl 86400 --uses 1 --endpoint https://192.168.1.50:8443

# On joining machine (Remote): submit join request
orbit join --invitation "orbit-invitation:v1?token=...&folder=...&endpoint=https%3A%2F%2F192.168.1.50%3A8443" \
  --root ~/Notes --label "Travel Laptop"

# On primary workstation: review and approve
orbit requests list --status pending
orbit requests approve --request req-xxxx --alias "Travel Laptop"
```

---

## Daily Operations & CLI Parity

All GUI file management, conflict triage, and storage actions have full command-line parity:

```sh
# Browse workspace contents and search paths
orbit browse --folder <id> --path /docs --limit 50
orbit search --folder <id> --query "quarterly"

# Durable file mutations (journaled and crash-consistent)
orbit mkdir --folder <id> --path /docs/archive
orbit import --folder <id> --src report.pdf --dest /docs/archive/report.pdf
orbit move --folder <id> --src /docs/old.md --dest /docs/new.md
orbit delete --folder <id> --path /docs/temp --recursive

# Conflicts and Historical Restore
orbit conflicts --folder <id> --json
orbit restore --folder <id> --path note.txt --source <version-id> --preview

# Storage, GC, and Bounded Pruning
orbit storage --folder <id>
orbit storage gc --folder <id>
orbit maintenance prune --max-age 24h
```

*(Legacy syntax `filesync <command>` continues to operate identically).*

---

## Reproduce Release Experiments

Automated validation suites require Python 3 and Go:

```sh
# Run validation test suites
python3 -m unittest discover -s scripts/validation -p 'test_*.py'

# Multi-host pilot harness
python3 scripts/validation/three_host.py --laptop laptop --pi rpi --vps vps

# Systemd user service lifecycle validation
python3 scripts/validation/service_lifecycle.py --hosts laptop rpi vps \
  --output /tmp/orbit-service-lifecycle

# Benchmark suite (10,000 files & 1 GiB payload)
go run scripts/benchmark_suite.go --small-files 10000 --large-mib 1024 --repetitions 1
```

---

## Operator Runbooks

Detailed runbooks for system administration, disaster recovery, and networking:

- [Linux Installation and Lifecycle Setup](docs/runbooks/install.md)
- [Safe Uninstallation & Data Preservation](docs/runbooks/uninstall.md)
- [Headless Device Pairing & Administration](docs/runbooks/headless-pairing.md)
- [Private Network Configuration & Reachability](docs/runbooks/private-network.md)
- [Database Recovery & Identity Reset](docs/runbooks/database-recovery.md)
- [Lost Device Replacement & Decommissioning](docs/runbooks/lost-device-replacement.md)
- [Binary Rollback & Downgrade Safety](docs/runbooks/rollback.md)
- [Storage Accounting & Full Disk Remediation](docs/runbooks/full-disk.md)
