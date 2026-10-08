# E00 commands — 2026-10-08

Revision: `22f4bee` (clean tree before E00), Go `go1.27.1-X:nodwarf5 linux/amd64`,
dev PC Arch Linux kernel 7.2.8, 16 CPUs, 62 GiB. All state in marked disposable
roots (`testkit.NewDisposable`); daemons stopped through `ValidateDestructiveTarget`.

## Host facts (read-only)

```sh
sha256sum ~/.local/bin/orbit bin/orbit               # PC: both d86070095e30…c5476d
sha256sum dist/orbit-v2.0.0-linux-amd64.tar.gz       # 48dc4a8acc54…eb7d5a
tar -xzOf dist/orbit-v2.0.0-linux-amd64.tar.gz orbit | sha256sum   # 11123a78d5a5…f4c888
ssh laptop 'sha256sum ~/.local/bin/orbit'            # d86070095e30…c5476d (same as PC)
ssh rpi 'sha256sum /usr/bin/orbit; dpkg-query -W orbit'   # 0bc710deb0a8…fe6c72, orbit 2.0.0
ss -ltnp | grep ':8080 '                             # PC: 127.0.0.1:8080 held by java
systemctl get-default; systemctl --user is-active graphical-session.target
loginctl show-user "$USER" -p Linger; loginctl list-seats   # on PC, laptop (ssh), Pi (ssh)
```

All three report `orbit 2.0.0` commit `8abd497`. The PC and laptop binaries are the
repository `bin/orbit` build (`install.sh` falls back to it); the tarball's
binary differs in checksum at the same commit.

## Reproductions

```sh
go test -list '^TestOnboardingE' ./internal/... ./cmd/orbit/... ./tests/...   # 14 names
ORBIT_ONBOARDING_BASELINE=1 go test ./internal/terminal ./internal/scheduler ./internal/replication \
  -run '^TestOnboardingE00' -count=1 -v                     # logs/model-unit.log: 9 FAIL (expected)
ORBIT_ONBOARDING_BASELINE=1 go test ./tests/terminal -run '^TestOnboardingE00F' -count=1 -v
                                                            # logs/process.log: 4 FAIL (expected)
ORBIT_ONBOARDING_BASELINE=1 go test ./tests/terminal -run '^TestOnboardingE00RelayJoinWaitNamesAndModes$' \
  -count=1 -v -timeout 15m                                  # logs/relay-join-wait.log: FAIL (expected), 206 s
go test ./internal/terminal ./internal/scheduler ./internal/replication ./tests/terminal \
  -run '^TestOnboardingE00' -count=1 -v                     # logs/ordinary-skip.log: 14 SKIP, exit 0
go vet ./internal/terminal ./internal/scheduler ./internal/replication ./tests/terminal; gofmt -l internal tests
```

`make check` was not run: E00 changes only opt-in tests and documentation, and
the ordinary run of every touched package skips the new cases (exit 0).
