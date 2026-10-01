# File Sync

File Sync synchronizes selected folders between trusted Linux devices. A Go
agent watches ordinary files, retains captured versions in immutable storage,
and exchanges them over authenticated HTTPS. SQLite stores causal history,
working-copy state, transfer progress, and recovery journals.

Independent offline edits remain separate heads until you explicitly resolve
reviewed versions. You can select a version, supply a manual merge, keep copies,
or restore retained content as a new change. A VPS can store and forward a
version while its author is offline; it has no conflict authority.

Replicas hold readable content. TLS protects transfers. Captured-version
protection assumes supported local filesystems and storage that honors flushes;
it does not cover every intermediate editor write, disk loss, or arbitrary
writes through descriptors held across replacement. See the
[approved scope](docs/portfolio-scope.md) and [persistence contract](docs/persistence.md).

**Release validation remains in progress.** The [status tracker](docs/implementation/status.md)
and [current evidence](docs/evidence/release-20261001/summary.md) distinguish
local tests, actual-host demonstrations, VM resets, benchmarks, and personal use.
The earlier 2026-09-24 estimated wire-savings figures have been withdrawn.

## Build and local demonstration

Use the Go toolchain pinned in `go.mod` (1.27.1), Make, and Linux with the
required descriptor-relative filesystem calls. The binary embeds the web UI
and needs no Node runtime. Rebuilding frontend assets requires the pinned npm
lockfile: `cd web`, `npm ci`, then `npm run build`.

```sh
make build build-arm64
./bin/filesync version
make demo
make check
make test-race
```

`make demo` creates disposable loopback peers and cleans up its own paths.
`make check` runs formatting, vet, unit/model/integration/process-fault checks,
builds both architectures, and generates packages. The VM reset experiment is
an explicit separate command, never part of ordinary installation or checks.

## Setup and daily operation

A folder ID is 64 hexadecimal characters, shared by its enrolled devices.
Start with a dedicated folder and state directory:

```sh
STATE="$PWD/filesync-state"
ROOT="$PWD/filesync-notes"
FOLDER_ID="0101010101010101010101010101010101010101010101010101010101010101"
mkdir -m 0700 "$ROOT"
./bin/filesync init --state "$STATE"
./bin/filesync register --state "$STATE" --folder "$FOLDER_ID" --root "$ROOT"
./bin/filesync identity --state "$STATE" --certificate
./bin/filesync scan --state "$STATE" --folder "$FOLDER_ID"
./bin/filesync serve --state "$STATE" --peer-listen 127.0.0.1:8443 \
  --control-listen 127.0.0.1:8080
```

Exchange public certificates and key pins out of band, approve the same folder
membership on every device, and configure reachable peer addresses. The
[local demo](scripts/local_demo.go) shows two-peer pairing; the
[actual-host harness](scripts/validation/three_host.py) uses canonical
three-member approval. See the [installation runbook](docs/runbooks/install.md)
for packages and user-service configuration.

Use `conflicts --json` to inspect reviewed head IDs and their token. A selection
names the path, selected `author:counter`, reviewed IDs, and current token:

```sh
./bin/filesync resolve select --state "$STATE" --folder "$FOLDER_ID" \
  --path note.txt --selected "$SELECTED_VERSION" --reviewed "$REVIEWED_VERSIONS" \
  --head-token "$HEAD_TOKEN" --idempotency-key "$OPERATION_ID"
./bin/filesync restore --state "$STATE" --folder "$FOLDER_ID" \
  --path note.txt --source "$HISTORICAL_VERSION" --preview --json
```

`filesync <command> -h` lists actual flags. CLI/control/UI share the same engine
operations. Per-peer status distinguishes saved, stored, applied, conflicted,
and unavailable contents; a VPS receipt does not establish a Pi receipt.

## Reproduce release experiments

Python 3 is required for validation orchestration. Remote commands require
existing SSH aliases and a Python interpreter; each run creates fresh private
marked roots and tracks its exact processes. Existing pilot folders and other
services are preserved. Roots are retained for inspection.

```sh
python3 -m unittest discover -s scripts/validation -p 'test_*.py'
go run scripts/three_host_pilot.go --laptop laptop --pi rpi --vps vps
go run scripts/benchmark_suite.go --small-files 10000 --large-mib 1024 --repetitions 1
python3 scripts/validation/service_lifecycle.py --output /tmp/filesync-lifecycle-evidence
python3 scripts/validation/abrupt_reset.py --kernel /path/to/vmlinuz \
  --output /tmp/filesync-reset-evidence
```

The reset experiment requires QEMU/KVM, `mkfs.ext4`, and a kernel with built-in
virtio-blk/ext4/devtmpfs support. It discards guest dirty caches and tests
selected production boundaries, under the recorded virtual storage assumptions.
It does not demonstrate a physical Pi power cut.

The synthetic benchmark counts TLS-bearing TCP stream bytes in both directions.
Its full-file baseline also uses mutual TLS, hashes and durably installs files,
and skips unchanged files after hashing. TCP/IP and SSH headers are excluded.
Different history/storage work and warm filesystem caches limit timing comparisons.
Raw runs, failed experiments, and negative results remain in the evidence.
An automated demo does not establish the required personal-use pilot.

See [the case study](docs/case-study.md), [architecture](docs/architecture.md),
[protocol](docs/protocol.md), [verification](docs/verification.md), and
[operator runbooks](docs/runbooks/install.md).
