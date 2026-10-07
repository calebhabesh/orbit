# W17 commands

All commands ran on 2026-10-07. Logs are in `reproduction/`, `provenance/` and
`lifecycle-vm/`. The step runner printed `seconds=0.000` because `bc` is not
installed; wall times come from the step start timestamps in
`reproduction/steps.txt`.

## W16 package provenance

```sh
git clone -q <repo> prov && cd prov
git checkout -q d253825
git diff d253825 a8ef7e6 --binary | git apply     # tree verified equal to a8ef7e6
make package COMMIT=d253825
sha256sum dist/orbit-v1.0.0-linux-{arm64,amd64}.tar.gz
# 596ceece…18fb arm64, fa24501b…7546 amd64: identical to W16 runs 17–20
```

## Lifecycle drill (disposable KVM guest)

```sh
curl -fsSLO https://cloud.debian.org/images/cloud/trixie/latest/SHA512SUMS
curl -fsSLO https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.qcow2
grep ' debian-13-genericcloud-amd64.qcow2$' SHA512SUMS | sha512sum -c -   # OK
make package
python3 scripts/validation/service_boot_vm.py --image <qcow2> \
  --archive dist/orbit-v1.0.0-linux-amd64.tar.gz --work <new dir> --output <new dir>
```

Runs 1–3 failed and drove the fixes (see `results.json`); runs 4 and 5 passed;
run 5 used the final source.

## Focused checks after the fixes

```sh
go test -count=1 -run TestSelectedServiceAcceptsPackagedHomeSpecifier ./internal/control/   # red before the fix
go test -count=1 -race ./internal/control/... ./internal/terminal/... ./cmd/filesync/... ./tests/integration/
go test -count=1 -race -v ./tests/terminal -run 'TestTerminalT02'
```

## Clean reproduction (isolated snapshot 0c48429)

The snapshot is a commit object made from the working tree with a temporary
index (`GIT_INDEX_FILE=… git add -A; git write-tree; git commit-tree`). It is
not on any branch and was cloned into a scratch directory.

```sh
make GOFLAGS=-v check                                   # rc 0, 1,719 s (terminal 1,506.284 s)
go test -race -count=1 -timeout=45m -v ./...            # rc 0, 1,615 s, 21 packages
make demo                                               # rc 0
make package; diff SHA256SUMS.check SHA256SUMS.repeat   # identical
make package-orbit-net (twice); diff                    # identical
make test-terminal-release                              # rc 0, 146 s, 8 tests x2 under race
python3 -O -m unittest discover -s scripts/validation -p 'test_*.py' -v   # 36 OK
pgrep -a -f '/tmp/Test.*serve'                          # none before or after
```

## Final tree

```sh
git diff --name-only <snapshot> <final tree>   # docs/ and README.md only
make test-terminal-packages                    # rc 0; package includes runbooks/networking.md
gitleaks dir docs --redact                     # classified; no credential (see manifest)
python3 linkcheck.py <changed docs>            # 0 bad links/anchors
git diff --check
```
