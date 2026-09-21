# P00 commands and observed results

Commands ran from `<repo>` on 2026-09-20.

```text
$ go version
go version go1.27.1-X:nodwarf5 linux/amd64

$ go mod verify
all modules verified

$ make check
PASS: vet, internal tests, integration test, amd64 build, arm64 cross-build

$ make test-race
PASS: all current packages on linux/amd64

$ go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7
PASS: no diagnostics

$ git init <temporary-snapshot>; git commit; git clone <snapshot> <checkout>
$ make -C <checkout> check
PASS: fresh temporary checkout at /tmp/filesync-p00-checkout.dsWYe2/source

$ file bin/filesync bin/filesync-linux-arm64
amd64: statically linked ELF 64-bit x86-64
arm64: statically linked ELF 64-bit ARM aarch64

$ /tmp/qemu-aarch64-static --version
qemu-aarch64 version 7.2.0 (Debian 1:7.2+dfsg-1~bpo11+2)

$ /tmp/qemu-aarch64-static ./bin/filesync-linux-arm64 init --state <temporary-state>
initialized device ... in /tmp/filesync-p00-arm64.hzD0e8

$ sqlite3 <temporary-state>/metadata.sqlite \
    'PRAGMA user_version; SELECT count(*) FROM installation_metadata;'
1
0

$ stat -c '%a %n' <state> <state>/config.json <state>/metadata.sqlite
700 <state>
600 <state>/config.json
600 <state>/metadata.sqlite
```

The initial direct `docker run --platform linux/arm64 alpine:3.22 uname -m`
attempt failed with `exec format error` because host binfmt was unavailable.
The successful check invokes an extracted `qemu-aarch64-static` explicitly and
does not alter host binfmt registration.

Hosted GitHub Actions: **unexecuted**. The workflow exists and passed local
static validation; it cannot run against this uncommitted worktree without an
external push.
